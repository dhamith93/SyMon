#!/bin/bash
OLD=`awk '{print $5}' /sys/block/sdb/stat`
DT=1
sleep DT
NEW=`awk '{print $5}' /sys/block/sdb/stat`
./agent -custom -name='disk-writes' -unit='write-reqs' -value=$((($NEW-$OLD)/$DT))
