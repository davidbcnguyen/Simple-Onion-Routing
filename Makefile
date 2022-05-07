.PHONY: clean all coord router client client_direct_to_web test_website test_website_view_only tracing_server

all: coord router client client_direct_to_web test_website test_website_view_only tracing_server

coord:
	go build -o bin/coord ./cmd/coord

router:
	go build -o bin/router ./cmd/router

client:
	go build -o bin/client ./cmd/client

client_direct_to_web:
	go build -o bin/client_direct_to_web ./cmd/client_direct_to_web

test_website:
	go build -o bin/test_website ./test_website

test_website_view_only:
	go build -o bin/test_website_view_only ./test_website_view_only

tracing_server:
	go build -o bin/tracing_server ./cmd/tracing_server

clean:
	rm -f bin/*