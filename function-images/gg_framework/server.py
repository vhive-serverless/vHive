import logging

from flask import Flask, request

import helloworld_pb2
import helloworld_pb2_grpc

import json
import os
import sys
from base64 import b64decode, b64encode


curdir = os.path.dirname(__file__)
sys.path.append(curdir)
sys.path.append(os.path.join(curdir, 'common'))
os.environ['PATH'] = "{}:{}".format(curdir, os.environ.get('PATH', ''))

if not os.environ.get('GG_DIR'):
    os.environ['GG_DIR'] = "/tmp/_gg"

if not os.environ.get('GG_CACHE_DIR'):
    os.environ['GG_CACHE_DIR'] = "/tmp/_gg/_cache"

from ggpaths import GGPaths, GGCache
from common import is_executable, make_executable, run_command

app = Flask(__name__)

def is_hash_for_thunk(hash):
    return len(hash) > 0 and hash[0] == 'T'

@app.route('/', methods=["POST"])
def hello():
    event = json.loads(request.data)
    os.environ['GG_STORAGE_URI'] = event['storageBackend']

    thunks = event.get('thunks')
    timelog = event.get('timelog')

    for thunk_item in thunks:
        thunk_data = b64decode(thunk_item['data'])
        with open(GGPaths.blob_path(thunk_item['hash']), "wb") as fout:
            fout.write(thunk_data)

    executables_dir = os.path.join(curdir, 'executables')
    if os.path.exists(executables_dir):
        for exe in os.listdir(executables_dir):
            blob_path = GGPaths.blob_path(exe)
            exe_path = os.path.join(executables_dir, exe)

            if not os.path.exists(blob_path):
                shutil.copy(exe_path, blob_path)
                make_executable(blob_path)

    os.system("rm -rf /tmp/thunk-execute.*")

    command = ["gg-execute-static",
                "--get-dependencies",
                "--put-output",
                "--cleanup"]

    if timelog:
        command += ["--timelog"]

    return_code, stdout = run_command(command + [x['hash'] for x in thunks])

    executed_thunks = []

    for thunk in thunks:
        outputs = []

        for output_tag in thunk['outputs']:
            output_hash = GGCache.check(thunk['hash'], output_tag)

            if not output_hash:
                return json.dumps({
                        'returnCode': return_code,
                        'stdout': stdout
                    })
            
            data = None
            if is_hash_for_thunk(output_hash):
                with open(GGPaths.blob_path(output_hash), 'rb') as tin:
                    data = b64encode(tin.read()).decode('ascii')

            outputs += [{
                    'tag': output_tag,
                    'hash': output_hash,
                    'size': os.path.getsize(GGPaths.blob_path(output_hash)),
                    'executable': is_executable(GGPaths.blob_path(output_hash)),
                    'data': data
            }]

        executed_thunks += [{
                'thunkHash': thunk['hash'],
                'outputs': outputs
        }]

    msg = {
            'returnCode': 0,
            'stdout': '',
            'executedThunks': executed_thunks
    }

    return msg



if __name__ == '__main__':
    app.run(host="0.0.0.0", port=50051, debug=True)
