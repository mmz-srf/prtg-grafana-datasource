# PRTG Datasource

![Marketplace Version](https://img.shields.io/badge/dynamic/json?logo=grafana&query=$.version&url=https://grafana.com/api/plugins/swisstxt-prtg-datasource&label=Version&prefix=v&color=F47A20) ![Marketplace downloads](https://img.shields.io/badge/dynamic/json?logo=grafana&query=$.downloads&url=https://grafana.com/api/plugins/swisstxt-prtg-datasource&label=Downloads&color=F47A20) ![Grafana Version](https://img.shields.io/badge/dynamic/json?logo=grafana&query=$.grafanaDependency&url=https://grafana.com/api/plugins/swisstxt-prtg-datasource&label=Grafana&prefix=v&color=F47A20)

[PRTG](https://www.paessler.com/prtg) is Paessler's all-in-one infrastructure monitoring platform. It polls thousands of **sensors** across your network — ping, CPU load, traffic, disk space, custom scripts, and hundreds of other sensor types — and keeps track of their current status and historic values.


![Panel editor with a historic time series query](https://raw.githubusercontent.com/mmz-srf/prtg-grafana-datasource/refs/heads/main/src/img/screenshots/panel-editor-historic.png)


## Requirements

- Grafana **12.3.0** or later.
- PRTG with the **Application Server / new UI and API v2** activated (*PRTG Setup → Activate New UI And New API* — on by default from PRTG 25.2.106 onward).

## Getting started

1. Install the plugin and add a new **PRTG-Datasource** in Grafana.
2. Set the **Server URL** of your PRTG core server and choose an authentication mode (API key, or username & password).
3. In a panel, pick a query type — *Current value* or *Historic time series* — and select a sensor/channel via the hierarchy picker, or switch to regex match mode to target many at once.

## Contributing
We welcome contributions! Please see [DEVELOPMENT.md](https://github.com/mmz-srf/prtg-grafana-datasource/blob/main/DEVELOPMENT.md) for details on how to set up a development environment and submit changes.
