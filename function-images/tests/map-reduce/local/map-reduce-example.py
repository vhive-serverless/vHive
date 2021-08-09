import boto3
import json
import random
import resource
from io import StringIO ## for Python 3
import os
import time
import sys
from joblib import Parallel, delayed

# set aws credentials:
AWS_ID = os.getenv('AWS_ACCESS_KEY', "")
AWS_SECRET = os.getenv('AWS_SECRET_KEY', "")

# create an S3 session

s3_client = boto3.resource(
        service_name='s3',
        region_name=os.getenv("AWS_REGION", 'us-west-1'),
        aws_access_key_id=AWS_ID,
        aws_secret_access_key=AWS_SECRET
    )
# s3 = boto3.resource('s3')
# s3_client = boto3.client('s3')

# constants
INPUT_MAPPER_PREFIX = "artemiy/input/"
OUTPUT_MAPPER_PREFIX = "artemiy/task/mapper/"
INPUT_REDUCER_PREFIX = OUTPUT_MAPPER_PREFIX
OUTPUT_REDUCER_PREFIX = "artemiy/task/reducer/"

def write_to_s3(bucket, key, data, metadata):
    # s3_client.Bucket(bucket).put_object(Key=key, Body=data, Metadata=metadata)
    s3object = s3_client.Object(bucket_name=bucket, key=key)
    s3object.put(Body=data, Metadata=metadata)

def mapper(event):
    
    start_time = time.time()

    dest_bucket = event['destBucket']  # s3 bucket where the mapper will write the result
    src_bucket  = event['srcBucket']   # s3 bucket where the mapper will search for input files
    src_keys    = event['keys']        # src_keys is a list of input file names for this mapper
    job_id      = event['jobId']
    mapper_id   = event['mapperId']
    print(src_keys, mapper_id)
   
    # aggr 
    output = {}
    line_count = 0
    err = ''

    # INPUT CSV => OUTPUT JSON

    for key in src_keys:
        key = INPUT_MAPPER_PREFIX + key
        # response = s3_client.get_object(Bucket=src_bucket, Key=key)
        # response = s3_client.Object(bucket_name=src_bucket, key=key).get()
        obj = s3_client.Object(bucket_name=src_bucket, key=key)
        response = obj.get()
        contents = response['Body'].read().decode("utf-8") 
    
        for line in contents.split('\n')[:-1]:
            line_count +=1
            try:
                data = line.split(',')
                srcIp = data[0][:8]
                if srcIp not in output:
                    output[srcIp] = 0
                output[srcIp] += float(data[3])
            except getopt.GetoptError as e:
#                print (e)
                err += '%s' % e

    time_in_secs = (time.time() - start_time)
    #timeTaken = time_in_secs * 1000000000 # in 10^9 
    #s3DownloadTime = 0
    #totalProcessingTime = 0 
    pret = [len(src_keys), line_count, time_in_secs, err]
    mapper_fname = "%sjob_%s/map_%s" % (OUTPUT_MAPPER_PREFIX, job_id, mapper_id) 
    print(mapper_fname)
    metadata = {
                    "linecount":  '%s' % line_count,
                    "processingtime": '%s' % time_in_secs,
                    "memoryUsage": '%s' % resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
               }
    print ("metadata", metadata)
    write_to_s3(dest_bucket, mapper_fname, json.dumps(output), metadata)
    return pret

'''
Mapper debug
ev = {
   "srcBucket": "storage-module-test", 
   "destBucket": "storage-module-test", 
   "keys": ["part-00000"],
   "jobId": "0",
   "mapperId": 0,
     }
mapper(ev)
'''




def reducer(event):
    
    start_time = time.time()
    
    dest_bucket = event['destBucket']  # s3 bucket where the mapper will write the result
    src_bucket  = event['srcBucket']   # s3 bucket where the mapper will search for input files
    reducer_keys = event['keys']    # reducer_keys is a list of input file names for this mapper
    job_id = event['jobId']
    r_id = event['reducerId']
    n_reducers = event['nReducers']
    
    # aggr 
    results = {}
    line_count = 0

    # INPUT JSON => OUTPUT JSON

    # Download and process all keys
    for key in reducer_keys:
        key = INPUT_REDUCER_PREFIX + "job_" + job_id + "/" + key
        print(key)
        # response = s3_client.get_object(Bucket=src_bucket, Key=key)
        response = s3_client.Object(bucket_name=src_bucket, key=key).get()
        contents = response['Body'].read().decode("utf-8")

        try:
            for srcIp, val in json.loads(contents).items():
                line_count +=1
                if srcIp not in results:
                    results[srcIp] = 0
                results[srcIp] += float(val)
        except:
            e = sys.exc_info()[0]
            print (e)

    time_in_secs = (time.time() - start_time)
    #timeTaken = time_in_secs * 1000000000 # in 10^9 
    #s3DownloadTime = 0
    #totalProcessingTime = 0 
    pret = [len(reducer_keys), line_count, time_in_secs]
    print ("Reducer ouputput", pret)

    if n_reducers == 1:
        # Last reducer file, final result
        fname = "%sjob_%s/result" % (OUTPUT_REDUCER_PREFIX, job_id)
    else:
        fname = "%sjob_%s/reducer_%s" % (OUTPUT_REDUCER_PREFIX, job_id, r_id)
    
    metadata = {
                    "linecount":  '%s' % line_count,
                    "processingtime": '%s' % time_in_secs,
                    "memoryUsage": '%s' % resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
               }

    write_to_s3(dest_bucket, fname, json.dumps(results), metadata)
    return pret

'''
Reducer debug
ev = {
    "srcBucket": "storage-module-test", 
    "destBucket": "storage-module-test",
    "keys": ["map_0"],
    "nReducers": 1,
    "jobId": "0",
    "reducerId": 0, 
}
reducer(ev)
'''


NUM_MAPPERS = 64 # can't be more than 2215 

def driver():
    map_ev = {
       "srcBucket": "storage-module-test", 
       "destBucket": "storage-module-test", 
       "keys": ["part-00000"],
       "jobId": "0",
       "mapperId": 0,
         }
    map_tasks = [] 
    for i in range(NUM_MAPPERS):
        map_tasks.append(map_ev.copy())
    for i in range(NUM_MAPPERS):
        print(i)
        map_tasks[i]['keys'] = ["part-" + str(i).zfill(5)]
        map_tasks[i]['mapperId'] = i

    # parallel running of mappers fails in pickle, probably due to very large output
#    Parallel(n_jobs=2)(delayed(mapper)(i) for i in map_tasks)
    
    for task in map_tasks:
        mapper(task)

    reduce_input_keys = ["map_" + str(x) for x in range(NUM_MAPPERS)]

    ev = {
        "srcBucket": "storage-module-test", 
        "destBucket": "storage-module-test",
        "keys": reduce_input_keys,
        "nReducers": 1,
        "jobId": "0",
        "reducerId": 0, 
        }
    reducer(ev)

driver()