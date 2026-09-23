# Installing the PRTG Datasource Plugin

This guide covers installing the **PRTG-Datasource** plugin (`swisstxt-prtg-datasource`) on a self-hosted Grafana instance or on Grafana Cloud, and configuring it to talk to a real PRTG server.

## Prerequisites

- **Grafana 12.3.0 or later** (see `grafanaDependency` in [src/plugin.json](src/plugin.json)).
- **PRTG with API v2 activated**: in PRTG, go to *Setup → Activate New UI And New API* (this installs the PRTG "Application Server" in the background and restarts the PRTG core server). From PRTG 25.2.106 onward this is on by default for new installations. Every endpoint this plugin uses is still marked "Experimental" by Paessler.
- Either a **PRTG API key**, or a **PRTG username and password**, for the plugin to authenticate with.

## Getting the plugin

Builds are published as GitHub Releases of this repository, produced by [.github/workflows/release.yml](.github/workflows/release.yml) whenever a version tag (`vX.Y.Z`) is pushed. Download the `.zip` asset attached to the release you want from the repository's **Releases** page.

By default, that build is **unsigned** — the release workflow's signing step is commented out until someone completes the one-time signing setup described in [DEVELOPMENT.md](DEVELOPMENT.md#distributing-your-plugin). This matters for where you can install it (see below):

- **Self-hosted Grafana**: works today, unsigned, as long as you explicitly allow this plugin ID to load unsigned (covered below).
- **Grafana Cloud**: does **not** load unsigned plugins at all. The plugin must first be signed with a **private** signature scoped to your Grafana Cloud stack's URL — see [Installing on Grafana Cloud](#installing-on-grafana-cloud).

## Installing on a self-hosted Grafana instance

### Option A: Docker

Mount the extracted plugin into Grafana's plugin directory, and allow it to load unsigned:

```bash
docker run -d \
  -p 3000:3000 \
  -v "$(pwd)/swisstxt-prtg-datasource:/var/lib/grafana/plugins/swisstxt-prtg-datasource" \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=swisstxt-prtg-datasource \
  grafana/grafana:12.3.0
```

Or let Grafana's own entrypoint fetch and unpack the release zip for you, via [`GF_INSTALL_PLUGINS`'s custom-URL form](https://grafana.com/docs/grafana/latest/setup-grafana/configure-docker/) (`<url>;<plugin-id>`):

```bash
docker run -d \
  -p 3000:3000 \
  -e GF_INSTALL_PLUGINS="https://github.com/mmz-srf/prtg-grafana-datasource/releases/download/vX.Y.Z/<asset>.zip;swisstxt-prtg-datasource" \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=swisstxt-prtg-datasource \
  grafana/grafana:12.3.0
```

Replace `vX.Y.Z` and `<asset>.zip` with the actual release tag and asset filename.

### Option B: Manual install (package/binary Grafana)

1. Extract the release zip into Grafana's plugin directory (typically `/var/lib/grafana/plugins` on a Linux package install; `data/plugins` for a binary install):
   ```bash
   unzip swisstxt-prtg-datasource-*.zip -d /var/lib/grafana/plugins/swisstxt-prtg-datasource
   ```
2. Allow this plugin ID to load unsigned. Either set the environment variable `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=swisstxt-prtg-datasource`, or add it to `grafana.ini`:
   ```ini
   [plugins]
   allow_loading_unsigned_plugins = swisstxt-prtg-datasource
   ```
3. Restart Grafana.
4. Under *Administration → Plugins and data → Plugins*, confirm **PRTG-Datasource** shows up (it won't appear in the catalog search since it isn't published there — look under "Installed").

## Installing on Grafana Cloud

Grafana Cloud refuses to load unsigned plugins outright, so this plugin needs a **private signature** first — a signature scoped to one or more specific Grafana instance URLs (your Cloud stack's URL) rather than a public catalog listing. At a high level:

1. Whoever maintains this repo completes the signing setup in [DEVELOPMENT.md](DEVELOPMENT.md#distributing-your-plugin): a Grafana Cloud API key with the `PluginPublisher` role, the plugin ID prefixed with that Cloud account's slug, and the release workflow's signing step enabled with `--rootUrls` covering your stack's URL (see Grafana's [plugin signing docs](https://grafana.com/developers/plugin-tools/publish-a-plugin/sign-a-plugin) for the exact, current process — it involves registering the plugin once via *Grafana Cloud → Org settings → My Plugins* before it can be signed).
2. Once a signed release zip exists, it's installed the same way any private plugin is added to a Cloud stack — see Grafana's current [plugin installation docs](https://grafana.com/docs/grafana/latest/administration/plugin-management/plugin-install/) and [Grafana Cloud plugin docs](https://grafana.com/docs/grafana-cloud/introduction/find-and-use-plugins/) for the exact click-path, since this is a Grafana Cloud policy/UI detail that can change independently of this plugin.
3. If your PRTG server is in a network Grafana Cloud can't reach directly, see [DEVELOPMENT.md's Private Data Source Connect section](DEVELOPMENT.md#connecting-to-a-private-prtg-server-from-grafana-cloud-pdc) once the plugin and datasource are set up.

If you only need this plugin on one or two self-hosted Grafana instances (not Grafana Cloud), skip signing entirely and use [Installing on a self-hosted Grafana instance](#installing-on-a-self-hosted-grafana-instance) instead — it's the simpler path.

## Configuring the datasource

Once the plugin is installed (either path above) and Grafana has restarted:

1. Go to *Connections → Data sources → Add new data source*, and search for **PRTG-Datasource**.
2. Fill in the settings:

   | Field | Notes |
   | --- | --- |
   | **Server URL** | Your PRTG core server's base URL, e.g. `https://prtg.example.com`. |
   | **Authentication** | `API Key`, or `Username & Password`. |
   | **API Key** *(API Key mode)* | A PRTG API key with read access to the objects you want to query. |
   | **Username** / **Password** *(Username & Password mode)* | PRTG login credentials; the plugin logs in via PRTG's session endpoint. |
   | **Skip TLS Verify** | Enable only if your PRTG server uses a self-signed certificate you can't otherwise trust. |
   | **Secure Socks Proxy** | Leave off unless you're using Grafana's/Grafana Cloud's [secure socks proxy or Private Data Source Connect](DEVELOPMENT.md#connecting-to-a-private-prtg-server-from-grafana-cloud-pdc) to reach a PRTG server in a private network. |

3. Click **Save & test**.

To provision this instead of clicking through the UI (e.g. for infra-as-code setups), see the example at [provisioning/datasources/datasources.yml](provisioning/datasources/datasources.yml):

```yaml
apiVersion: 1
datasources:
  - name: 'PRTG'
    type: 'swisstxt-prtg-datasource'
    access: proxy
    jsonData:
      serverUrl: 'https://prtg.example.com'
      authMode: 'apiKey'
      tlsSkipVerify: false
    secureJsonData:
      apiKey: '<your PRTG API key>'
```

## Verifying the installation

**Save & test** performs a real, authenticated call against PRTG and reports one of:

| Message | Meaning |
| --- | --- |
| "Successfully connected to the PRTG API v2" | Everything works. |
| "Authentication failed: ..." | The API key, or username/password, is wrong. |
| "PRTG API v2 endpoint not found. Make sure the PRTG \"Application Server\" and the new UI/API are activated..." | PRTG API v2 isn't activated yet on the target server — see [Prerequisites](#prerequisites). |
| "Unable to reach the PRTG server: ..." | Network/DNS/TLS issue reaching **Server URL** — check the address, firewall rules, and (if applicable) PDC/proxy setup. |
| "PRTG server unavailable: ..." / "PRTG API error: ..." | PRTG returned an error of its own (e.g. temporarily overloaded); the message includes PRTG's own error text. |

(See [pkg/plugin/health.go](pkg/plugin/health.go) for exactly how these are derived.)

## Further reading

- [DEVELOPMENT.md](DEVELOPMENT.md) — building this plugin from source, local development against a mock PRTG server, and connecting to a private PRTG server via PDC.
- [README.md](README.md) — what this plugin does and its feature overview.
- [PRTG API v2 overview](https://www.paessler.com/support/prtg/api/v2/overview/index.html) — the REST API this plugin talks to.
- [Grafana plugin management docs](https://grafana.com/docs/grafana/latest/administration/plugin-management/) — the authoritative, up-to-date reference for installing and signing Grafana plugins in general.
