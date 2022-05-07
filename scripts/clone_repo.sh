#!/usr/bin/env bash

echo "Sending repo to servers";

IPS=("20.55.66.62" "20.51.186.185" "20.51.228.250" "20.51.228.49" "20.55.64.38" "20.83.180.95" "20.121.112.117")

for IP in "${IPS[@]}"; do
    scp -r ../../cpsc416_proj_mo3697_carsonh1_gkhui_ohryan55_bdai00_linushc "storadmin@$IP:~"
done