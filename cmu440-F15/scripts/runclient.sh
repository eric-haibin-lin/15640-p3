#!/bin/bash

if [ "$#" != "1" ]; then
    echo "This script creates a client runner connected with master server"
    echo "Usage: runclient.sh -masterPort 1234"
    exit 1
fi 

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

# Build the student's client node implementation.
# Exit immediately if there was a compile-time error.
go install github.com/cmu440-F15/paxosapp/runners/crunner
if [ $? -ne 0 ]; then
   echo "FAIL: code does not compile"
   exit $?
fi

# Build the test binary to use to test the student's client node implementation.
# Exit immediately if there was a compile-time error.
# go install github.com/cmu440-F15/clientapp/tests/clienttest
# if [ $? -ne 0 ]; then
   # echo "FAIL: code does not compile"
   # exit $?
# fi

# Pick random ports between [10000, 20000).
NODE_PORT0=$(((RANDOM % 10000) + 10000))
# NODE_PORT1=$(((RANDOM % 10000) + 10000))
# NODE_PORT2=$(((RANDOM % 10000) + 10000))
# TESTER_PORT=$(((RANDOM % 10000) + 10000))
# PROXY_PORT=$(((RANDOM & 10000) + 10000))
# CLIENT_TEST=$GOPATH/bin/clienttest
CLIENT_NODE=$GOPATH/bin/crunner
# ALL_PORTS="${NODE_PORT0},${NODE_PORT1},${NODE_PORT2}"

##################################################

# Start client node.
${CLIENT_NODE} -port=$NODE_PORT0 -masterPort=$1  &
CLIENT_NODE_PID0=$!
sleep 1

# ${CLIENT_NODE} -ports=${ALL_PORTS} -N=3 -id=1 -pxport=${PROXY_PORT} -proxy=0 2 &
# CLIENT_NODE_PID1=$!
# sleep 1

# ${CLIENT_NODE} -ports=${ALL_PORTS} -N=3 -id=2 -pxport=${PROXY_PORT} -proxy=0 2 &
# CLIENT_NODE_PID2=$!
# sleep 1

# Start clienttest. 
# ${CLIENT_TEST} -port=${TESTER_PORT} -clientports=${ALL_PORTS} -N=3 -nodeport=${NODE_PORT0} -pxport=${PROXY_PORT}

# Kill client node.
kill -9 ${CLIENT_NODE_PID0}
# kill -9 ${CLIENT_NODE_PID1}
# kill -9 ${CLIENT_NODE_PID2}
wait ${CLIENT_NODE_PID0} 2> /dev/null
# wait ${CLIENT_NODE_PID1} 2> /dev/null
# wait ${CLIENT_NODE_PID2} 2> /dev/null
