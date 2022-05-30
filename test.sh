#!/bin/bash

go build . && SB8200_HOST=192.168.100.1 SB8200_USER=admin SB8200_PASSWORD="<PASSWORD>" ./sb8200-exporter

