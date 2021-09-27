#!/bin/bash
OLD=`awk '{print $1}' /sys/block/sdb/stat`
DT=1
sleep DT
NEW=`awk '{print $1}' /sys/block/sdb/stat`
./agent -custom -name='disk-reads' -unit='read-reqs' -value=$((($NEW-$OLD)/$DT))
