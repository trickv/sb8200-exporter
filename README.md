# Arris Cable Modem Exporter

This is a Prometheus exporter for Arris cable modems. I only own an SB8200 but
it may work with others. ~If you own a different model please reach out and I'd
love to get it working there.~

~If anybody wants to send me a different model to test I'd also be thankful.~

I no longer have Comcast so no longer use this code and have no way to test it if I wanted. do still reach out if you want to get this working and have trouble.

## Supported Devices

- Arris SB8200

## Setup

Compile and install wherever you prefer.

I recomend the following systemd unit:

```
[Unit]
Description=SB8200 Modem Exporter
Wants=network-online.target
After=network-online.target

[Service]
User=sb8200_exporter
Group=sb8200_exporter
Type=simple
Environment=SB8200_HOST=192.168.100.1
Environment=SB8200_USER=admin
Environment=SB8200_PASSWORD=[PASSWORD]
ExecStart=/usr/local/bin/sb8200-exporter
RuntimeMaxSec=3h
Restart=always

[Install]
WantedBy=multi-user.target
```

Then configure Prometheus with the new data source:

```
# prometheus.yml
scrape_configs:
  - job_name: 'sb8200_exporter'
    scrape_interval: 1m
    scrape_timeout: 20s
    static_configs:
      - targets: ['localhost:9143']
```

### Dashboard

The `example_dashboard.json` file has a useful starting point for a grafana
dashboard. You'll need to adjust this to your specific device(s).

### Docker

```
docker run -d -p 9143:9143 \
  -e SB8200_HOST=192.168.100.1 \
  -e SB8200_USER=admin \
  -e SB8200_PASS=password \
  sb8200-exporter
```
