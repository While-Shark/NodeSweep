# Notification destinations

Notifications are optional and disabled by default. Configure the destination only in the hub's protected `0600` configuration file, then restart it. The dashboard shows whether a webhook is configured and its format, never the destination or embedded secret.

```json
"webhookURL": "https://your-public-receiver.example/hooks/secret",
"webhookFormat": "event"
```

Supported formats:

| Format | Destination | Payload |
| --- | --- | --- |
| `event` (default) | A trusted HTTPS receiver | Existing structured event JSON |
| `slack` | Slack incoming webhook | Plain text block and fallback text |
| `discord` | Discord incoming webhook | Text, empty allowed mentions and suppressed embeds |

Platform formats bound metadata, remove mention/markup syntax, and disable link unfurling or embeds. Cleanup-failure messages contain a task identifier, never raw task errors, credentials or cleanup file lists. Threshold messages include node identity and mount information; send them only to trusted destinations.

The same public-IP validation, TLS verification, DNS-rebinding protection, no-proxy connection, redirect refusal and bounded timeout apply to every format. Non-2xx responses are failures. There is no automatic cleanup-failure delivery replay: a crash or uncertain network result can leave delivery unknown, and the task record remains authoritative.

## Email through an HTTPS relay

For email, use `event` with your trusted public HTTPS relay. Configure the recipients and SMTP credentials in that relay, outside NodeSweep. Map `kind`, `resolved`, `name`, `path`, `percent`, `task` and `at` into a fixed subject/body template, authenticate the webhook's secret path, and validate the event schema. Do not interpret event data as shell commands, header values or templates. Keep message size, delivery rates and duplicate handling bounded. Confirm delivery with your relay before relying on it.

NodeSweep has no embedded SMTP server or direct SMTP sender. This forwarding path keeps mail credentials out of its browser and Agent protocol; private-only relay addresses are rejected by the webhook transport.

References: [Slack incoming webhooks](https://docs.slack.dev/messaging/sending-messages-using-incoming-webhooks/), [Slack text objects](https://docs.slack.dev/reference/block-kit/composition-objects/text-object/), [Discord webhooks](https://docs.discord.com/developers/resources/webhook).
