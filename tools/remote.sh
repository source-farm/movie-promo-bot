#!/bin/bash

# Перенаправление данных между двумя портами на локальной машине.
# Скрипт ждёт установки соединения на оба порта, т.е. он сам не
# инициирует соединение.

if [[ "$#" -ne 2 ]]; then
    echo `basename "$0"` "needs two arguments: <port_1> <port_2>"
    exit 1
fi

PORT_1=$1
PORT_2=$2

while true; do
    socat -d TCP-LISTEN:$PORT_1,reuseaddr TCP-LISTEN:$PORT_2,reuseaddr
    sleep 1
done
