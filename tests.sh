#!/bin/sh
go test -race -json -coverprofile=coverage.txt -covermode atomic -v $(go list ./... | grep -v /mock) 2>&1 | tee /tmp/gotest.log | gotestfmt 
exit_status=$?

if [ $exit_status -ne 0 ]; then
  echo "FAILED!"
  exit 1
else  
    go tool cover -func=coverage.txt
fi
