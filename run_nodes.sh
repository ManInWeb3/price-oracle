#!/bin/bash

if [ -z "${NUM_NODES}" ]; then
    echo "Error: Environment variable 'NUM_NODES' is not set."
    echo "You can set it with: export NUM_NODES=5"
    echo "Or run the script in the follwoing way: NUM_NODES=5 run_nodes.sh"
    exit 1
fi

MainBin="./oracle"

# Generate a new Rendezvous string on each run
# This is required only to make connections bootstrapping faster in DEV envs
# Can be fixed with bootstrap nodes
Rendezvous=$(tr -dc A-Za-z0-9 </dev/urandom | head -c 16; echo)

for (( i=0; i<$NUM_NODES; i+=1 )); do
  $MainBin -n node$i -rendezvousString $Rendezvous  > >(tee -a node$i.log) 2>&1 &
done

stop_bg_nodes() {
  ps axu|grep oracle|awk '{system("kill " $2)}'
}

trap 'stop_bg_nodes' EXIT

echo "Script is running. Press Ctrl+C to exit."

while true; do
  sleep 1
done
