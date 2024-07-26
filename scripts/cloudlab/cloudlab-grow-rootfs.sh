#!/bin/sh

#
# If user wants to resize the rootfs to the max, try to do that.
#
set -x
RESIZEROOT=0

if [ `id -u` -ne 0 ] ; then
    echo "This script must be run as root" 1>&2
    exit 1
fi

if [ -z "$RESIZEROOT" ]; then
    echo "ERROR: must define RESIZEROOT to the new total size (GB) you want for the rootfs!"
    exit 0
fi
if [ -z "$IMPOTENT" ]; then
    IMPOTENT=0
fi
if [ -z "$NODELETE" ]; then
    NODELETE=0
fi

# Remove any existing temp files.
rm -fv /tmp/sfdisk.orig /tmp/sfdisk.new /tmp/sfdisk.new \
    /tmp/sfdisk.nextstart /tmp/sfdisk.parts-to-delete

# Find the root partition's parent disk.
eval `lsblk -n -P -b -o NAME,FSTYPE,MOUNTPOINT,PARTTYPE,PARTUUID,TYPE,PKNAME,SIZE | grep 'MOUNTPOINT="/"'`
ROOTPARENT=$PKNAME
ROOT=$NAME
if [ -z "$ROOTPARENT" -o -z "$ROOT" ]; then
    echo "ERROR: unable to find root device or root parent disk; aborting!"
    exit 1
fi
# Find root partition number.
ROOTPARTNO=`echo "$ROOT" | sed -ne "s/^${ROOTPARENT}p\([0-9]*\)$/\1/p"`
if [ ! $? -eq 0 -o -z "$ROOTPARTNO" ]; then
    ROOTPARTNO=`echo "$NAME" | sed -ne "s/^${ROOTPARENT}\([0-9]*\)$/\1/p"`
fi
if [ -z "$ROOTPARTNO" ]; then
    echo "ERROR: could not determine the root partition number; aborting!"
    exit 1
fi

# Save off the original partition table, and create a new one to modify.
sfdisk -d /dev/$ROOTPARENT > /tmp/sfdisk.orig
cp -p /tmp/sfdisk.orig /tmp/sfdisk.new

if [ $NODELETE -eq 0 ]; then
    # Swapoff all swap devices if we are not impotent; they will be
    # removed.
    for dev in `blkid -t TYPE=swap | cut -d: -f1 | xargs` ; do
	if [ ! $IMPOTENT -eq 1 ]; then
	    swapoff $dev
	    if [ ! $? -eq 0 ]; then
		echo "ERROR: failed to swapoff $dev; aborting!"
		exit 1
	    fi
	fi
    done

    # Figure out which partitions to remove.  We remove any partition on
    # the rootparent with FSTYPE="" and MOUNTPOINT="" and
    # PARTUUID=(0fc63daf-8483-4772-8e79-3d69d8477de4|00000000-0000-0000-0000-000000000000|0657FD6D-A4AB-43C4-84E5-0933C84B4F4F|0x83|0x82|0x0).

    PARTS=""
    lsblk -a -n -P -b -o NAME,FSTYPE,MOUNTPOINT,PARTTYPE,PARTUUID,TYPE,PKNAME,SIZE | grep "PKNAME=\"${ROOTPARENT}\"" | while read line ; do
	eval "$line"
	if [ "$FSTYPE" != swap -a \( -n "$FSTYPE" -o -n "$MOUNTPOINT" \) ]; then
	    continue
	fi
	echo "$PARTTYPE" | grep -qEi '^(0fc63daf-8483-4772-8e79-3d69d8477de4|00000000-0000-0000-0000-000000000000|0657FD6D-A4AB-43C4-84E5-0933C84B4F4F|0x83|0x82|0x0)$'
	if [ ! $? -eq 0 ]; then
	    continue
	fi
	# Now extract the partition number (to feed to parted).  Partition
	# number is not reported by most Linux tools nor by sysfs, so we
	# have to extract via regexp.  Right now we only worry about nvme
	# devices (or any device that ends with a "p\d+"), and assume that
	# anything else is "standard".
	PARTNO=`echo "$NAME" | sed -ne "s/^${PKNAME}p\([0-9]*\)$/\1/p"`
	if [ ! $? -eq 0 -o -z "$PARTNO" ]; then
	    PARTNO=`echo "$NAME" | sed -ne "s/^${PKNAME}\([0-9]*\)$/\1/p"`
	fi
	if [ ! $? -eq 0 -o -z "$PARTNO" ]; then
	    continue
	fi
	PARTS="$PARTNO $PARTS"
	echo $PARTNO >> /tmp/sfdisk.parts-to-delete
    done

    if [ -e /tmp/sfdisk.parts-to-delete ]; then
	PARTS=`cat /tmp/sfdisk.parts-to-delete | xargs`
	rm -f /tmp/sfdisk.tmp
	cat /tmp/sfdisk.new | while read line ; do
	    delete=0
	    for part in $PARTS ; do
		echo "$line" | grep -q "^/dev/${ROOTPARENT}$part :"
		if [ $? -eq 0 ]; then
		    delete=1
		    break
		fi
	    done
	    if [ $delete -eq 0 ]; then
		echo "$line" >> /tmp/sfdisk.tmp
	    fi
	done
	diff -u /tmp/sfdisk.new /tmp/sfdisk.tmp
	mv /tmp/sfdisk.tmp /tmp/sfdisk.new
    fi
fi

#
# Now we need to figure out the max sector we can end on.  If there is a
# partition further up the disk, we can't stomp it.
#
DISKSIZE=`sfdisk -l /dev/$ROOTPARENT | sed -ne 's/^Disk.*, \([0-9]*\) sectors$/\1/p'`
ROOTSTART=`sfdisk -l -o device,start,end /dev/$ROOTPARENT | sed -ne "s|/dev/${ROOT} *\([0-9]*\) *\([0-9]*\)$|\1|p"`
ROOTEND=`sfdisk -l -o device,start,end /dev/$ROOTPARENT | sed -ne "s|/dev/${ROOT} *\([0-9]*\) *\([0-9]*\)$|\2|p"`
ROOTSIZE=`expr $ROOTEND - $ROOTSTART + 1`
# First, we find the max size of the new root partition in sectors.  If
# we find a partition with a start greater than ROOTEND, that value -
# 2048 is the new end.  Otherwise, it is DISKSIZE - 2048.
nextstart=$DISKSIZE
cat /tmp/sfdisk.new | grep "^/dev" | while read line ; do
    nstart=`echo $line | sed -ne "s|/dev/[^ ]* *: *start= *\([0-9]*\),.*$|\1|p"`
    if [ -z "$nstart" ] ; then
	continue
    fi
    if [ $nstart -gt $ROOTSTART -a $nstart -lt $nextstart ]; then
	nextstart=$nstart
	echo $nextstart > /tmp/sfdisk.nextstart
    fi
done
if [ -e /tmp/sfdisk.nextstart -a -s /tmp/sfdisk.nextstart ]; then
    nextstart=`cat /tmp/sfdisk.nextstart`
fi
align=0
if [ ! `expr $nextstart \% 2048` -eq 0 ]; then
    align=2048
fi
maxsize=`expr $nextstart - $align - $ROOTSTART`
# Sanitize the size.  We only support GB.
RESIZEROOT=`echo "$RESIZEROOT" | sed -ne 's/^\([0-9]*\)[^0-9]*$/\1/p'`
if [ -z "$RESIZEROOT" ]; then
    echo "ERROR: could not determine size of root disk $ROOTPARENT; aborting!"
    exit 1
fi
if [ $RESIZEROOT -eq 0 ]; then
    newsize=$maxsize
else
    usersectors=`expr $RESIZEROOT \* 1024 \* 1024 \*  1024 / 512`
    if [ $usersectors -gt $maxsize ]; then
	newsize=$maxsize
    else
	newsize=$usersectors
    fi
fi
if [ -z "$newsize" ]; then
    echo "ERROR: failed to calculate new root partition size; aborting!"
    exit 1
fi
if [ $newsize -lt $ROOTSIZE ]; then
    echo "ERROR: newsize ($newsize) less than current root size ($ROOTSIZE); aborting!"
    exit 1
fi
if [ $newsize -lt 2048 ]; then
    echo "WARNING: cannot expand root partition; skipping!"
    exit 0
fi

# Finally, edit the sfdisk.new file to change the root device's size.
if [ $newsize -ne $ROOTSIZE ]; then
    echo "Expanding the /dev/$ROOT partition"
    cat /tmp/sfdisk.new | while read line ; do
        echo "$line" | grep -q "^/dev/${ROOT} :"
        if [ $? -eq 0 ]; then
            echo "$line" | sed -e "s|^\(/dev/${ROOT} :.*\)\(size= *[0-9]*,\)\(.*\)$|\1size=${newsize}\3|" >> /tmp/sfdisk.tmp
        else
            echo "$line" >> /tmp/sfdisk.tmp
        fi
    done
    mv /tmp/sfdisk.tmp /tmp/sfdisk.new

    diff -u /tmp/sfdisk.orig /tmp/sfdisk.new

    if [ $IMPOTENT -eq 1 ]; then
        exit 0
    fi

    sfdisk --force /dev/$ROOTPARENT < /tmp/sfdisk.new
    partprobe /dev/$ROOTPARENT
fi

echo "Resizing the root filesystem"
resize2fs /dev/$ROOT
if [ ! $? -eq 0 ]; then
    echo "ERROR: failed to resize /dev/$ROOT filesystem; aborting!"
    exit 1
fi

echo "Resized /dev/$ROOT."

exit 0
