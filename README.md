# mindav

A small WebDAV server backed by Google Cloud Storage.

It exposes a GCS bucket over WebDAV, namespacing each user's files under
`u/<user>/` in the bucket. Authentication is delegated to a fronting
oauth2-proxy via the `X-Auth-Request-User` header; the special user
`kixtoken@` allows non-browser clients (e.g. KeePass) to authenticate with a
Mattermost personal access token supplied as the basic-auth password.

## Configuration

| Env var          | Required | Default            | Description                          |
|------------------|----------|--------------------|--------------------------------------|
| `GCS_BUCKET`     | yes      | —                  | Target GCS bucket                    |
| `PORT`           | no       | `8080`             | Listen port                          |
| `MATTERMOST_URL` | no       | `https://kix.co.il`| Mattermost API base for token auth   |

Credentials use Application Default Credentials (Workload Identity on GKE).

The service is served under `/v1/webdav` (the ingress rewrites the public
`/webdav/(.*)` path to `/v1/webdav/$1`).

## Develop

```sh
go test ./...
go build .
```
