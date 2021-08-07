#!/bin/bash                                                                               
echo -e "Emter desired object size in bytes"
read objectsize
truncate -s $objectsize $objectsize.txt
mc cp $objectsize.txt myminio/mybucket
rm $objectsize.txt
