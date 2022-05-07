#!/usr/bin/env bash

SERVER_ADDR="20.83.180.95"
CLIENT_PORT="50051"
CLIENT_1_ADDR="20.121.112.117"
CLIENT_2_ADDR="20.127.70.164"
CLIENT_3_ADDR="20.127.53.223"
ITERATIONS=10
echo "Sending wgets $ITERATIONS times."

echo $1
trap "exit" INT TERM ERR
trap "kill 0" EXIT
for ((i = 0; i < $ITERATIONS; i++))
do
    sleep 1
    if [ $1 == 1 ]
    then
      wget "$CLIENT_1_ADDR:$CLIENT_PORT/$SERVER_ADDR" &
    fi
    if [ $1 == 2 ]
    then
      wget "$CLIENT_2_ADDR:$CLIENT_PORT/$SERVER_ADDR" &
    fi
    if [ $1 == 3 ]
    then
      wget "$CLIENT_3_ADDR:$CLIENT_PORT/$SERVER_ADDR" &
    fi
done
wait

echo "Done"
