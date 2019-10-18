#!/bin/bash

# Перенаправление данных от удалённого хоста на локальный порт и обратно.
# Инициатором соединения как на удалённый хост так и на локальный порт является
# сам скрипт.

if [[ "$#" -ne 3 ]]; then
    echo `basename "$0"` "needs three arguments: <remote_addr> <remote_port> <local_port>"
    exit 1
fi

REMOTE_ADDR=$1
REMOTE_PORT=$2
LOCAL_PORT=$3

while true; do
    socat -d -d TCP:$REMOTE_ADDR:$REMOTE_PORT TCP:localhost:$LOCAL_PORT
    sleep 1
done
