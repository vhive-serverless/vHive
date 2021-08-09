import boto3
import json
import random
import resource
from io import StringIO ## for Python 3
import time
import sys
from joblib import Parallel, delayed


# constants
INPUT_MAPPER_PREFIX = "artemiy/input/"
OUTPUT_MAPPER_PREFIX = "artemiy/task/mapper/"
INPUT_REDUCER_PREFIX = OUTPUT_MAPPER_PREFIX
OUTPUT_REDUCER_PREFIX = "artemiy/task/reducer/"

def write_to_s3(bucket_obj, key, data, metadata):
    bucket_obj.put_object(Key=key, Body=data, Metadata=metadata)

def read_from_s3(s3_client, src_bucket, key):
    response = s3_client.get_object(Bucket=src_bucket, Key=key)
    return response

def mapper(event):

    dest_bucket = event['destBucket']  # s3 bucket where the mapper will write the result
    src_bucket  = event['srcBucket']   # s3 bucket where the mapper will search for input files
    src_keys    = event['keys']        # src_keys is a list of input file names for this mapper
    job_id      = event['jobId']
    mapper_id   = event['mapperId']
    n_reducers  = event['nReducers']   # must be power of 2
#    print(src_keys, mapper_id)
   
    # aggr 
    output = {}
    line_count = 0
    err = ''

    # create an S3 session
    # Note that this can be done once for all mappers/reducers as part fo the
    # GreeterService object init by setting appropriate transferTime
    # If S3 session is initialized locally by each mapper/reducer, it can considerably i
    # increase their compute time. Ask Dmitrii and Artemiy for mode detail
    s3 = boto3.resource('s3')
    s3_client = boto3.client('s3')

    # INPUT CSV => OUTPUT JSON

    start_time = 0
    for key in src_keys:
        key = INPUT_MAPPER_PREFIX + key
        response = s3_client.get_object(Bucket=src_bucket, Key=key)
        start_time = time.time()
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

    shuffle_output = []
    for i in range(n_reducers):
        reducer_output = {}
        shuffle_output.append(reducer_output)
    for srcIp in output.keys():
        reducer_num = hash(srcIp) & (n_reducers - 1)
        shuffle_output[reducer_num][srcIp] = output[srcIp]

    time_in_secs = (time.time() - start_time)

    write_tasks = []
    for to_reducer_id in range(n_reducers):
        mapper_fname = "%sjob_%s/shuffle_%s/map_%s" % (OUTPUT_MAPPER_PREFIX, job_id, to_reducer_id, mapper_id) 
#        print(mapper_fname)
        metadata = {
                        "linecount":  '%s' % line_count,
                        "processingtime": '%s' % time_in_secs,
                        "memoryUsage": '%s' % resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
                   }
        write_tasks.append( (s3.Bucket(dest_bucket), mapper_fname, json.dumps(shuffle_output[to_reducer_id]), metadata) )

    Parallel(backend="threading", n_jobs=n_reducers)(delayed(write_to_s3)(*i) for i in write_tasks)

    pret = [len(src_keys), line_count, time_in_secs, err]
    print("mapper" + str(mapper_id), pret)
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

    # create an S3 session
    # Note that this can be done once for all mappers/reducers as part fo the
    # GreeterService object init by setting appropriate transferTime
    # If S3 session is initialized locally by each mapper/reducer, it can considerably i
    # increase their compute time. Ask Dmitrii and Artemiy for mode detail
    s3 = boto3.resource('s3')
    s3_client = boto3.client('s3')

    # INPUT JSON => OUTPUT JSON

    # Download and process all keys
    read_tasks = []
    for key in reducer_keys:
        key = INPUT_REDUCER_PREFIX + "job_" + job_id + "/shuffle_" + str(r_id) + "/" + key
        read_tasks.append((s3_client, src_bucket, key))
    responses = Parallel(backend="threading", n_jobs=len(read_tasks))(delayed(read_from_s3)(*i) for i in read_tasks)

    time_in_secs = 0
    start_time = time.time()

    for response in responses:
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

    time_in_secs += (time.time() - start_time)

    #timeTaken = time_in_secs * 1000000000 # in 10^9 
    #s3DownloadTime = 0
    #totalProcessingTime = 0 
    pret = [len(reducer_keys), line_count, time_in_secs]
    print ("Reducer" + str(r_id), pret)

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

    write_to_s3(s3.Bucket(dest_bucket), fname, json.dumps(results), metadata)
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
NUM_REDUCERS = 16 # must be power of 2 and smaller than NUM_MAPPERS 

def driver():
    map_ev = {
       "srcBucket": "storage-module-test", 
       "destBucket": "storage-module-test", 
       "keys": ["part-00000"],
       "jobId": "0",
       "mapperId": 0,
       "nReducers": NUM_REDUCERS,
         }
    map_tasks = [] 
    for i in range(NUM_MAPPERS):
        map_tasks.append(map_ev.copy())
        map_tasks[i]['keys'] = ["part-" + str(i).zfill(5)]
        map_tasks[i]['mapperId'] = i

    for task in map_tasks:
        mapper(task)


    reduce_input_keys = ["map_" + str(x) for x in range(NUM_MAPPERS)] # this list is the same for 
                                                                      # all reducers as each of them has to read 
                                                                      # a shuffle result from each mapper
    reduce_ev = {
        "srcBucket": "storage-module-test", 
        "destBucket": "storage-module-test",
        "keys": reduce_input_keys,
        "nReducers": NUM_REDUCERS,
        "jobId": "0",
        "reducerId": 0, 
        }
    reducer_tasks = [] 
    for i in range(NUM_REDUCERS):
        reducer_tasks.append(reduce_ev.copy())
        reducer_tasks[i]['reducerId'] = i

    for task in reducer_tasks:
        reducer(task)

driver()
