# Grafana data source plugin template

This template is a starting point for building a Data Source Plugin for Grafana.

## What are Grafana data source plugins?

Grafana supports a wide range of data sources, including Prometheus, MySQL, and even Datadog. There’s a good chance you can already visualize metrics from the systems you have set up. In some cases, though, you already have an in-house metrics solution that you’d like to add to your Grafana dashboards. Grafana Data Source Plugins enables integrating such solutions with Grafana.

## Getting started

### Backend

1. Update [Grafana plugin SDK for Go](https://grafana.com/developers/plugin-tools/key-concepts/backend-plugins/grafana-plugin-sdk-for-go) dependency to the latest minor version:

   ```bash
   go get -u github.com/grafana/grafana-plugin-sdk-go
   go mod tidy
   ```

2. Build plugin backend binaries for Linux, Windows and Darwin:

   ```bash
   mage -v
   ```

3. List all available Mage targets for additional commands:

   ```bash
   mage -l
   ```

### Frontend

1. Install dependencies

   ```bash
   npm install
   ```

2. Build plugin in development mode and run in watch mode

   ```bash
   npm run dev
   ```

3. Build plugin in production mode

   ```bash
   npm run build
   ```

4. Run the tests (using Jest)

   ```bash
   # Runs the tests and watches for changes, requires git init first
   npm run test

   # Exits after running all the tests
   npm run test:ci
   ```

5. Spin up a Grafana instance and run the plugin inside it (using Docker)

   ```bash
   npm run server
   ```

6. Run the E2E tests (using Playwright)

   ```bash
   # Spins up a Grafana instance first that we tests against
   npm run server

   # If you wish to start a certain Grafana version. If not specified will use latest by default
   GRAFANA_VERSION=11.3.0 npm run server

   # Starts the tests
   npm run e2e
   ```

7. Run the linter

   ```bash
   npm run lint

   # or

   npm run lint:fix
   ```

## Local development with a mock PRTG server

There is no official PRTG container -- PRTG (Paessler) is Windows-only, closed-source, license-bound software. So `npm run server` (`docker compose up --build`) also starts a small standalone **mock PRTG APIv2 server** (`mock-prtg`, source in [pkg/mockprtg](pkg/mockprtg) / [cmd/mockserver](cmd/mockserver), built from [Dockerfile.mockserver](Dockerfile.mockserver)) alongside the Grafana dev container. It serves a seeded, realistic-looking object hierarchy (probes → groups → devices → sensors → channels: ping, CPU load, traffic, memory, disk free) with procedurally generated timeseries data, on `http://localhost:8080`.

The provisioned `prtg-datasource` datasource ([provisioning/datasources/datasources.yml](provisioning/datasources/datasources.yml)) already points at it, so Grafana at `http://localhost:3000` should work against it out of the box -- no manual setup needed.

Configuration (environment variables on the `mock-prtg` service, all optional):

| Variable        | Default          | Purpose                                       |
| --------------- | ---------------- | ---------------------------------------------- |
| `PORT`          | `8080`           | listen port                                    |
| `MOCK_API_KEY`  | `mock-api-key`   | bearer token accepted for the `apiKey` auth mode |
| `MOCK_USERNAME` | `mock-user`      | username accepted by `POST /session` (`credentials` auth mode) |
| `MOCK_PASSWORD` | `mock-password`  | password accepted by `POST /session`           |

If you change `MOCK_API_KEY`, update `secureJsonData.apiKey` in `provisioning/datasources/datasources.yml` to match.

Quick manual check without Grafana:

```bash
curl http://localhost:8080/api/v2/experimental/groups
```

This mock is unrelated to [pkg/plugin/fakeserver_test.go](pkg/plugin/fakeserver_test.go), a smaller, test-only PRTG double used by this repo's own Go unit tests.

## Connecting to a private PRTG server from Grafana Cloud (PDC)

If your PRTG server sits in a network Grafana Cloud can't reach directly, use [Private Data Source Connect (PDC)](https://grafana.com/docs/grafana-cloud/connect-externally-hosted/private-data-source-connect/) -- no inbound firewall access needed. The agent that makes this work, `grafana/pdc-agent`, is an official Grafana binary/Docker image; nothing needs to be built for it.

This plugin's backend builds its HTTP client via the Grafana SDK's `settings.HTTPClientOptions(ctx)` ([pkg/plugin/datasource.go](pkg/plugin/datasource.go)), which is what gives PDC's secure-socks tunnel a hook to route this datasource's requests through -- so PDC works once it's set up below, with no further plugin changes required.

**1. Create a PDC network in Grafana Cloud** -- *Connections > Private data source connect > Add new*. The **Configuration Details** page then shows three values: `GCLOUD_PDC_SIGNING_TOKEN`, `GCLOUD_HOSTED_GRAFANA_ID`, `GCLOUD_PDC_CLUSTER`.

**2. Run the pdc-agent somewhere with network access to your PRTG server** (typically inside that private network -- not necessarily on the machine running this repo's `npm run server`):

- Using this repo's optional docker-compose service: create a `.env` file here with the three values (`GCLOUD_PDC_SIGNING_TOKEN=...`, `GCLOUD_HOSTED_GRAFANA_ID=...`, `GCLOUD_PDC_CLUSTER=...`), then
  ```bash
  docker compose --profile pdc up -d pdc-agent
  docker compose logs -f pdc-agent
  ```
  It's opt-in via the `pdc` [Compose profile](https://docs.docker.com/compose/how-tos/profiles/) -- a plain `docker compose up` / `npm run server` never starts it.
- Or the plain `docker run` from Grafana's docs:
  ```bash
  docker run --name pdc-agent \
    -e GCLOUD_PDC_SIGNING_TOKEN=<token> \
    -e GCLOUD_HOSTED_GRAFANA_ID=<instance-id> \
    -e GCLOUD_PDC_CLUSTER=<cluster> \
    grafana/pdc-agent:latest
  ```
- Or the [pdc-agent binary](https://github.com/grafana/pdc-agent/releases/latest) directly on a Linux/Windows host (requires OpenSSH ≥ 9.2; Docker/Kubernetes images bundle a compatible OpenSSH already).

Watch the logs for a successful tunnel connection before continuing.

**3. Point the datasource at PRTG through the tunnel.** On this plugin's datasource settings page in Grafana Cloud: pick your PDC network under **Private data source connection**, and set **Server URL** to how PRTG is reachable *from where the agent runs* (an internal hostname/IP, e.g. `https://prtg.internal.example.com`) -- not a public address. Save & Test.

The config editor also has its own **Secure Socks Proxy** switch (`jsonData.enableSecureSocksProxy`) -- Grafana Cloud's PDC network picker normally sets this for you; it's there mainly for parity with self-hosted Grafana's generic [secure socks proxy](https://grafana.com/docs/grafana/latest/setup-grafana/configure-grafana/proxy/) feature.

(This assumes the PRTG datasource plugin itself is already installed in your Grafana Cloud stack -- a separate, standard [private plugin installation](https://grafana.com/docs/grafana-cloud/developer-resources/plugin-development/) step not covered here.)

# Distributing your plugin

When distributing a Grafana plugin either within the community or privately the plugin must be signed so the Grafana application can verify its authenticity. This can be done with the `@grafana/sign-plugin` package.

_Note: It's not necessary to sign a plugin during development. The docker development environment that is scaffolded with `@grafana/create-plugin` caters for running the plugin without a signature._

## Initial steps

Before signing a plugin please read the Grafana [plugin publishing and signing criteria](https://grafana.com/legal/plugins/#plugin-publishing-and-signing-criteria) documentation carefully.

`@grafana/create-plugin` has added the necessary commands and workflows to make signing and distributing a plugin via the grafana plugins catalog as straightforward as possible.

Before signing a plugin for the first time please consult the Grafana [plugin signature levels](https://grafana.com/legal/plugins/#what-are-the-different-classifications-of-plugins) documentation to understand the differences between the types of signature level.

1. Create a [Grafana Cloud account](https://grafana.com/signup).
2. Make sure that the first part of the plugin ID matches the slug of your Grafana Cloud account.
   - _You can find the plugin ID in the `plugin.json` file inside your plugin directory. For example, if your account slug is `acmecorp`, you need to prefix the plugin ID with `acmecorp-`._
3. Create a Grafana Cloud API key with the `PluginPublisher` role.
4. Keep a record of this API key as it will be required for signing a plugin

## Signing a plugin

### Using Github actions release workflow

If the plugin is using the github actions supplied with `@grafana/create-plugin` signing a plugin is included out of the box. The [release workflow](./.github/workflows/release.yml) can prepare everything to make submitting your plugin to Grafana as easy as possible. Before being able to sign the plugin however a secret needs adding to the Github repository.

1. Please navigate to "settings > secrets > actions" within your repo to create secrets.
2. Click "New repository secret"
3. Name the secret "GRAFANA_API_KEY"
4. Paste your Grafana Cloud API key in the Secret field
5. Click "Add secret"

#### Push a version tag

To trigger the workflow we need to push a version tag to github. This can be achieved with the following steps:

1. Run `npm version <major|minor|patch>`
2. Run `git push origin main --follow-tags`

## Learn more

Below you can find source code for existing app plugins and other related documentation.

- [Basic data source plugin example](https://github.com/grafana/grafana-plugin-examples/tree/master/examples/datasource-basic#readme)
- [`plugin.json` documentation](https://grafana.com/developers/plugin-tools/reference/plugin-json)
- [How to sign a plugin?](https://grafana.com/developers/plugin-tools/publish-a-plugin/sign-a-plugin)
