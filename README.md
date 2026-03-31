# Stockyard Trough

API cost monitor. Track request counts, response sizes, and estimated costs for any HTTP API.

## What it does

Trough is a reverse proxy that sits in front of any API and tracks what you spend. Point your app at Trough instead of the API directly, and you get per-request cost tracking, daily breakdowns, and spend alerts.

Works with any HTTP API: Twilio, Stripe, SendGrid, AWS, or your own internal services.

## Features

- **Per-request tracking** — log every request with endpoint, method, response size, latency, and estimated cost
- **Cost rules** — define pricing rules per endpoint (e.g., Twilio SMS = $0.0079/request)
- **Daily breakdowns** — cost by day, by endpoint, by upstream service
- **Spend alerts** — webhook when daily or monthly spend exceeds a threshold
- **Multi-upstream** — proxy to multiple APIs through one Trough instance
- **CSV export** — download cost reports for finance teams
- **Single binary** — Go + embedded SQLite, no external dependencies
- **Self-hosted** — request data never leaves your infrastructure

## Quick start

```bash
curl -fsSL https://stockyard.dev/trough/install.sh | sh
trough serve --upstream https://api.twilio.com --port 8770
```

## Pricing

- **Free:** 10,000 requests/month, 1 upstream
- **Pro ($14/mo):** Unlimited requests, unlimited upstreams, alerts, CSV export

## Part of Stockyard

Trough extends the cost tracking engine from [Stockyard](https://stockyard.dev), the self-hosted LLM infrastructure platform.

## License

Apache 2.0
