#!/bin/bash

if [ -z $GOPATH ]; then
echo "FAIL: GOPATH environment variable is not set"
exit 1
fi

if [ -n "$(go version | grep 'darwin/amd64')" ]; then
GOOS="darwin_amd64"
elif [ -n "$(go version | grep 'linux/amd64')" ]; then
GOOS="linux_amd64"
else
echo "FAIL: only 64-bit Mac OS X and Linux operating systems are supported"
exit 1
fi

# Build mrunner
go install github.com/cmu440-F15/paxosapp/runners/mrunner
if [ $? -ne 0 ]; then
echo "FAIL: code does not compile"
exit $?
fi

# Build the student's paxos node implementation.
# Exit immediately if there was a compile-time error.
go install github.com/cmu440-F15/paxosapp/runners/mrunner
if [ $? -ne 0 ]; then
echo "FAIL: code does not compile"
exit $?
fi

MONITOR_NODE=$GOPATH/bin/mrunner

$MONITOR_NODE 