#! /bin/bash
# MIT License
#
# Copyright (c) 2020 Shyam Jesalpura and EASE lab
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

if [ -z $1 ] ; then
    echo "Parameters missing"
    echo "USAGE: setup_crontab.sh <location of config file>"
    exit -1
fi

# Config file format
# KEY="kjbcjvakcjdc"
# NUM="2"
# LABEL="nightly"

source $1

TMPFILE=$(mktemp)

#write out current crontab
crontab -l > $TMPFILE
#echo new cron into cron file
echo "00 00 * * * shutdown -r 0" >> $TMPFILE
echo "@reboot /start_runners.sh $NUM $KEY $LABEL"
#install new cron file
crontab $TMPFILE