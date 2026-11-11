# Recognise Drawings in the browser, not on a server

A Drawing is recognised on the Owner's phone: nando-web runs a small convolutional network (the Owner's HanCharPI model, retrained on ETL9's 2,965 kanji) with a JavaScript ML runtime, and asks nando-server only which Candidates are stored Kanji, through `GET /api/japanese/kanji?kanji=…`. We chose this over a recognition service because, with the model shrunk to a few MB (global average pooling instead of a 2,048-unit dense layer), the phone can recognise a Drawing in milliseconds, after every stroke, offline, at no cost, and with nothing new to deploy, secure or keep warm.

## Considered Options

- **A Python service on Cloud Run** (`nando-kanji-ml`): costs nothing in the free tier and adds nothing to the phone, but it is a second service with its own deploy and Session check, and every stroke would need a round trip to it. What decided against it is that Draw could not work offline and that there would be one more service to run. Because it scales to zero (ADR 0001), the first Drawing after idle would also wait 2–5 s for a cold start. A warm-up request when Draw opens, or a minimum instance, would hide that, but the minimum instance is no longer free.
- **Inside nando-server**: rejected. Running the model needs a C library (ONNX Runtime or TensorFlow) through cgo, which breaks the static, `CGO_ENABLED=0` binary on `distroless/static`. A crash in that library would take every Module down with it. Writing the network's maths by hand in Go would avoid cgo, but every model change would mean rewriting and re-checking it.
- **KanjiCanvas** (MIT, pattern matching against stroke data, no ML): rejected because the Owner wants the trained model they built, and its accuracy on the Owner's handwriting is unknown.
- **Keep the HanCharPI model as it is**: rejected. It knows only ETL8G's 879 kanji, under half of Remembering the Kanji.

## Consequences

- ETL9's kanji are the JIS level 1 set, which leaves out some stored Kanji: a few jōyō kanji added in 2010 (such as 𠮟, 塡, 剝 and 頰) and possibly some of Heisig's volume 2. Those can never be a Candidate. We accept this; covering them would need training data beyond ETL9.
- The model is trained in its own repo, `nando-kanji-ml` (Keras, on the Owner's GPU). It is never trained in CI, and the ETL data never goes into git: it is too big, and its licence forbids redistributing it. The repo keeps the exported model and its top-8 accuracy on the ETL9 test set (the bar for shipping is 95%).
- A release of `nando-kanji-ml` opens a PR in nando-web that replaces `public/models/kanji/` (model, labels, version file and preprocessing fixtures). Only one model is kept; older ones live in git history.
- The service worker caches the model the first time the Owner opens Draw, not at install, so a few MB are only stored on the phone if Draw is used. The version file is always fetched from the network when online, never from the cache. Model and label files are requested with that version in their URL, and cached files with any other version are deleted, so labels and weights from different models never mix.
- Recognition works offline. Only the stored-Kanji lookup needs the server, so offline Candidates are shown as plain kanji.
- The image fed to the model is built from the Drawing's strokes (cropped, centred, fixed line width, 48×48), not from the screen's pixels. The same preprocessing exists in Python for training and in TypeScript for the app. `nando-kanji-ml` owns a set of fixtures (strokes in, expected 48×48 image out), and each repo tests its own version against them. The release PR copies the fixtures with the model, so both repos always test against the same ones.
- nando-web gains an ML runtime dependency, loaded only by Draw. Which one (TensorFlow.js, LiteRT.js or ONNX Runtime Web) is settled by trying each on the iPhone 17 for size and speed.
