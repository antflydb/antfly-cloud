# Connected sources beta

The first beta commands inspect saved source connections in an enrolled
organization. They use your existing Cloud login and selected organization and
instance. Availability is controlled by the server.

```sh
antfly-cloud login
antfly-cloud org use my-org
antfly-cloud instance use my-instance
antfly-cloud beta capabilities
antfly-cloud beta connections list
antfly-cloud beta connections list --json
```

Use `--org` and `--instance` to override the selection for a command. Login
refresh uses the existing session flow. No extra provider token or table
configuration is needed to inspect saved connections.

Connection listing currently includes GitHub and S3 summaries: ID, display name,
provider, authentication type, enabled state, saved status and optional validation
time. Saved status is not a live provider check. Credentials and provider
configuration are excluded. Viewing a connection does not share its content.

These commands are read-only. Connecting an account, creating or sharing a
source, starting a sync and connected search are not available in this increment.
`beta capabilities` reports those limitations explicitly. An unavailable beta or
older server returns an error, not an empty successful connection list.

The commands require ordinary Cloud permissions in addition to beta enrollment:
instance-read permission for capabilities and provider-credential-read permission
for connection listing. Enrollment grants neither permission nor data-plane
search access. The beta API and its Go SDK methods may change before general
availability.
