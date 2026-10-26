# Connect and sync source content

These commands require a compatible Colony API with source installation enabled.
Provider credentials belong to the hosted service. Users authorize Google
Workspace, Slack or Discord in their browser; no client secret or bot token is
entered into the CLI.

Select your organization and instance using the existing flags or saved context:

```sh
antfly-cloud login
antfly-cloud --org YOUR_ORG --instance YOUR_INSTANCE connections connect google-workspace --services drive,gmail,calendar
```

Approve access using the same Antfly user in the browser. The command waits for the
installation result and prints the connection ID. `--no-browser` prints the URL
without opening it; `--no-wait` returns the URL and flow ID immediately. An existing
Google connection can enable additional services with `--reconnect CONNECTION_ID`.
Drive, Gmail and Calendar share one Google Workspace connection.

List content and preview a selection, using the IDs returned by the server:

```sh
antfly-cloud connections list
antfly-cloud connections resources CONNECTION_ID --resource gmail
antfly-cloud sources preview --connection CONNECTION_ID --resource gmail --select LABEL_ID --attachments
```

For Drive select one folder (`--resource drive --select FOLDER_ID`); `--parent`
browses that folder's children when listing resources. Calendar uses
`--resource calendar --select CALENDAR_ID`. Gmail messages must match every selected
label. Resource listings may return a next cursor; pass it with `--cursor`.

Create a source in a dedicated destination table you have provisioned:

```sh
antfly-cloud sources add --connection CONNECTION_ID --resource gmail --select LABEL_ID --attachments --name customer-mail --table SOURCE_TABLE --every 15m
antfly-cloud sources list
antfly-cloud sources sync SOURCE_ID
```

The default sharing policy is private to the current user. `--shared` explicitly
shares with all readers authorized for the destination table. Attachments are
opt-in. Calendar Drive attachments also need the connection's Drive grant. The
server needs PDF/OCR extraction tools for those attachment formats.

Source creation queues the first sync; queue acceptance does not mean the content
is searchable. Check the source job/run and published catalog before searching.
If queuing fails after creation, the error includes the saved source ID so you can
retry `sources sync` without creating another source.

For Discord run `connections connect discord`, select the server in the provider
browser, then list `connections resources CONNECTION_ID --resource discord` and
use channel IDs with `sources preview` / `sources add`. Slack uses
`connections connect slack` and `--resource slack`. The managed Discord app needs
Message Content access and installation in the selected server. Each source stays
bound to its connection's server/workspace.

After successful publication, use the normal selected Cloud context with
`antfly-grep --cloud --files` and search the source alias. Provider authorization,
destination table access and source sharing are separate checks. Unsupported API
versions and unavailable installations return explicit errors.
