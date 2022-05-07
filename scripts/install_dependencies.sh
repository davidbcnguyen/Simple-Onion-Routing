#!/usr/bin/env bash

echo "Installing dependencies";

echo "Installing Go 1.16.7 to ~";

wget -P ~/ https://go.dev/dl/go1.16.7.linux-amd64.tar.gz
tar -xvzf ~/go1.16.7.linux-amd64.tar.gz -C ~/
echo "PATH=$PATH:~/go/bin" >> ~/.bashrc
source ~/.bashrc
export PATH=$PATH:~/go/bin

echo "Finished installing Go"

echo "Installing make"

sudo apt install make 

echo "Finished installing make"
