# MIT License
#
# Copyright (c) 2021 Michal Baczun and EASE lab
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

import pickle
import logging as log
import os
import boto3
import redis

#aws:
AWS_ID = os.getenv('AWS_ACCESS_KEY', "")
AWS_SECRET = os.getenv('AWS_SECRET_KEY', "")

#constants
S3 = "S3"
XDT = "XDT"
ELASTICACHE = "ELASTICACHE"


class Storage:
    # `init` initialises the storage modue. This function is used to provide information about
    # which service to use. If s3 is used, "bucket" is the bucket used for storage, and in the case
    # when elasticache is used "bucket" should be the redis URL and port.
    # Note that one must be on an AWS VPC (e.g. using EC2) to access elasticache.
    def __init__(self, service, bucket):
        self.transferType = service
        self.benchName = bucket
        self.s3_client = None
        self.elasticache_client = None
        if self.transferType == S3:
            self.s3_client = boto3.resource(
                service_name='s3',
                region_name=os.getenv("AWS_REGION", 'us-west-1'),
                aws_access_key_id=AWS_ID,
                aws_secret_access_key=AWS_SECRET
            )
        elif self.transferType == ELASTICACHE:
            self.elasticache_client = redis.Redis.from_url(bucket)

    # `put` uploads the payload to the storage service using the provided key
    def put(self, key, obj, doPickle = True):
        msg = "Driver uploading object with key '" + key + "' to " + self.transferType
        log.info(msg)
        pickled = obj
        if doPickle: 
            pickled = pickle.dumps(obj)
        if self.transferType == S3:
            s3object = self.s3_client.Object(bucket_name=self.benchName, key=key)
            s3object.put(Body=pickled)
        elif self.transferType == XDT:
            log.fatal("XDT is not supported")
        elif self.transferType == ELASTICACHE:
            self.elasticache_client.set(key, pickled)
        else:
            log.fatal("unsupported transfer type!")

        return key

    # `get` retrieves a payload corresponding to the provided key from the storage service.
    # An error will occur if the key is not prescent on the service.
    def get(self, key, doPickle = True):
        msg = "Driver gets key '" + key + "' from " + self.transferType
        log.info(msg)
        response = None
        if self.transferType == S3:
            obj = self.s3_client.Object(bucket_name=self.benchName, key=key)
            response = obj.get()
            if not doPickle:
                return response['Body'].read()
            else:
                return pickle.loads(response['Body'].read())
        elif self.transferType == XDT:
            log.fatal("XDT is not yet supported")
        elif self.transferType == ELASTICACHE:
            response = self.elasticache_client.get(key)
            if not doPickle:
                return response['Body'].read()
            else:
                return pickle.loads(response['Body'].read())
        else:
            log.fatal("unsupported transfer type!")