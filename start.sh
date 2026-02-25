#!/bin/bash
cd __REMOTE_DIR__

# --- Start askflow ---

pkill -f './askflow' 2>/dev/null
sleep 1
nohup ./askflow > askflow.log 2>&1 &
sleep 2
if ss -tlnp | grep -q ':8080 '; then
    echo SERVICE_OK
else
    # Check if config corruption caused the failure
    if grep -q "cipher: message authentication failed" askflow.log 2>/dev/null; then
        echo "Config file corrupted, removing and retrying..."
        rm -f data/config.json
        pkill -f './askflow' 2>/dev/null
        sleep 1
        nohup ./askflow > askflow.log 2>&1 &
        sleep 2
        if ss -tlnp | grep -q ':8080 '; then
            echo SERVICE_OK
        else
            echo SERVICE_FAIL
            tail -5 askflow.log
        fi
    else
        echo SERVICE_FAIL
        tail -5 askflow.log
    fi
fi
