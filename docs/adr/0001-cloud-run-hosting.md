# Host on Cloud Run in Tokyo, not on a Compute Engine VM

Nando runs as two Cloud Run services in `asia-northeast1`: `nando-web` (nginx serving the static build) on `app.<domain>` and `nando-server` (Go API) on `api.<domain>`, with data in a dedicated free MongoDB Atlas M0 cluster (Atlas project `nandoproject`, GCP `asia-northeast1`). We chose this over the Owner's familiar pattern of an e2-micro VM because, for a single-Owner app, Cloud Run's free tier makes compute effectively free, there's no in-use external IPv4 charge (~$3.65/month per running VM), Google manages HTTPS certificates for the mapped domain, and there's no OS to maintain.

## Considered Options

- **e2-micro VM + Caddy + domain**: compute is free-tier, but the static external IP is billed while the VM runs, the free tier is US-only (~120 ms from Japan), and certificates, OS and restarts become our job.
- **Scheduled VM shutdowns** to cut the IP charge: rejected because the server must be reachable at any hour to deliver Notifications and to serve the app.
- **A `nando` database in the existing US-East Atlas cluster** (shared with benkyo): rejected because every query from a Tokyo server would cross the Pacific (~150 ms each), and it would share benkyo's 512 MB quota.
- **Single origin** (`<domain>/api`): simpler (no CORS), but separate subdomains keep the two repos independently deployable; `app.` and `api.` are same-site, so the session cookie still works under Safari's cross-site cookie blocking.

## Consequences

- Cloud Run scales to zero, so the server cannot keep its own timer. Time-based work (Notifications) is driven by a Cloud Scheduler job calling a Google-signed (OIDC) `POST /internal/tick` every 30 minutes, at :00 and :30 in the Owner Time Zone. Notification times are therefore chosen in 30-minute steps, so each one is sent exactly when it was set for. Finer timing would only need a more frequent schedule, not a redesign.
- Cloud Run has no fixed egress IP, so Atlas network access is `0.0.0.0/0`; the database is protected by credentials held in Secret Manager.
- Cloud Run domain mapping was described by Google as preview-quality; if it becomes a problem, Firebase Hosting (or a load balancer) in front of the services is the fallback.
- The API needs CORS with credentials for `https://app.<domain>`, and the web app calls it with `credentials: "include"`.
