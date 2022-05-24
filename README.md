# Onion Routing Proxy

## Build
`go build`

`make all`

## Running
### Tracing
Before we can run the proxy, we must start the tracing server with

`./bin/tracing_server`
### Proxy
Once the tracing_server is up, we need to make sure that the values in the config folder are all correct. There can be multiple client and router configs in the format of:
> client_config1.json, client_config2.json, ... client_configN.json

> router_config1.json, router_config2.json, ... router_configN.json

First start the coordinator node which will act as a directory node

`./bin/coord`

Then the proxy routers can join with

`./bin/router n`

where n is the router config number

Then finally, once at least 3 routers are connected to the coord, the clients can start their proxy service with

`./bin/client n`

where n is the client config number