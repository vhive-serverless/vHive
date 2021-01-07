FROM ustiugov/lr_serving:var_workload as var_workload
COPY requirements.txt .
RUN	pip3 install --no-cache-dir -r requirements.txt

COPY *.py /

EXPOSE 50051

STOPSIGNAL SIGKILL

CMD ["/usr/bin/python", "/server.py"]
