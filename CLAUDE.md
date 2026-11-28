## Agent skills

### Issue tracker

Issues are tracked in this repo's GitHub Issues (via the `gh` CLI). See `docs/agents/issue-tracker.md`.

### Triage labels

Uses the five default triage labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Google Cloud

Nando runs in the GCP project **`benkyo-ferios`**, region **`asia-northeast1`** (Tokyo, ADR 0001): the Cloud Run services `nando-server` and `nando-web`, Secret Manager, Cloud Scheduler's tick and Cloud Logging. Use them in every `gcloud` command (`--project benkyo-ferios`, `--region` or `--location asia-northeast1`) instead of asking the Owner. The deploy workflows read the same values from the repo variables `GCP_PROJECT_ID` and `GCP_REGION`.

### Read-only access in cloud sessions

The environment provides the read-only service account `claude-reader@benkyo-ferios.iam.gserviceaccount.com` as a base64 key in `GCP_SA_KEY_B64`. Activate it once per session, keeping the key outside the repo:

```bash
mkdir -p -m 700 ~/.config/gcloud-sa && (umask 077; echo "$GCP_SA_KEY_B64" | base64 -d > ~/.config/gcloud-sa/nando-readonly.json)
env -u CLOUDSDK_AUTH_ACCESS_TOKEN gcloud auth activate-service-account --key-file ~/.config/gcloud-sa/nando-readonly.json
```

The environment sets a placeholder `CLOUDSDK_AUTH_ACCESS_TOKEN`, which overrides every other credential (`CREDENTIALS_MISSING`) and is set again in each new shell. Drop it on every call: `env -u CLOUDSDK_AUTH_ACCESS_TOKEN gcloud run services list --project benkyo-ferios --region asia-northeast1`. The account can read only; changes go through the deploy workflows or the Owner.
