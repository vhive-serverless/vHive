import os
import numpy as np
import matplotlib.pyplot as plt
import pandas as pd
import statistics

def plot_p99_percentile():
	hello_p = get_99th_percentile("hello_cold_start_latencies.txt")
	pyaes_p = get_99th_percentile("pyaes_cold_start_latencies.txt")
	rnn_p = get_99th_percentile("rnn_cold_start_latencies.txt")

	hello_np = get_99th_percentile("normal_hello_cold_start_latencies.txt")
	pyaes_np = get_99th_percentile("normal_pyaes_cold_start_latencies.txt")
	rnn_np = get_99th_percentile("normal_rnn_cold_start_latencies.txt")

	x = np.arange(3)
	y1 = [hello_np, pyaes_np, rnn_np]
	y2 = [hello_p, pyaes_p, rnn_p]
	width = 0.2

	fig = plt.figure(figsize = (10, 5))
	plt.bar(x-0.2, y1, color ='green', width = 0.4)
	plt.bar(x+0.2, y2, color ='blue', width = 0.4)
	plt.xticks(x, ['Helloworld', 'Pyaes', 'Rnn'])

	plt.title("99th percentile response time for cold start function response")
	plt.xlabel("Function")
	plt.ylabel("Latency (miliseconds)")
	plt.legend(["Default vHive snaps", "Enhanced vHive snaps"])
	#plt.show()
	plt.savefig('p99_latencies.png')

def get_99th_percentile(cold_start_file):
	response_latencies = []
	with open(cold_start_file) as file:
		lines = [line.rstrip() for line in file]
		for line in lines:
			if line.isdigit():
				miliseconds = int(line) / 10 ** 3
				response_latencies.append(miliseconds)
	file.close()

	result_latencies = []
	response_latencies.sort()
	[result_latencies.append(x) for x in response_latencies if x not in result_latencies]
	# print(response_latencies)

	out_file = "Processed_" + cold_start_file
	out = open(out_file,"w+")
	for latency in response_latencies:
		out.write(str(latency) + "\n")
	out.close()

	a = np.array(result_latencies)
	arr1 = np.round(a,decimals = 2)
	p_90 = np.percentile(a, 90)
	p_95 = np.percentile(a, 95)
	p_99 = np.percentile(a, 99)

	print(cold_start_file)
	# print(p_90)
	# print("------")
	# print(p_95)
	#print("------")
	#print(p_99)

	return p_99


def plot_pull_images_and_snap_files():
	pull_hello_times = []
	pull_pyaes_times = []
	pull_rnn_times = []

	with open("real_times_pull_images.txt") as file:
		lines = [line.rstrip() for line in file]
		i = 0
		while i < (len(lines) - 2):
			seconds_hello = float(lines[i])
			seconds_pyaes = float(lines[i + 1])
			seconds_rnn = float(lines[i + 2])

			# print(seconds_hello)
			# print(seconds_pyaes)
			# print(seconds_rnn)

			pull_hello_times.append(seconds_hello)
			pull_pyaes_times.append(seconds_pyaes)
			pull_rnn_times.append(seconds_rnn)

			i += 3

	file.close()


	hello_med = statistics.mean(pull_hello_times)
	pyaes_med = statistics.mean(pull_pyaes_times)
	rnn_med = statistics.mean(pull_rnn_times)

	functions = ['Helloworld', 'Pyaes', 'Rnn']
	values = [hello_med, pyaes_med, rnn_med]

	print(values)


def main():
	# os.system("head ./fulllocal_bench/hellowrld/latencies/rps* -n 1 >> hello_cold_start_latencies.txt")
	# os.system("head ./fulllocal_bench/pyaes/latencies/rps* -n 1 >> pyaes_cold_start_latencies.txt")
	# os.system("head ./fulllocal_bench/rnn/latencies/rps* -n 1 >> rnn_cold_start_latencies.txt")

	# os.system("head ./normal_vhive_bench/hellowrld/latencies/rps* -n 1 >> normal_hello_cold_start_latencies.txt")
	# os.system("head ./normal_vhive_bench/pyaes/latencies/rps* -n 1 >> normal_pyaes_cold_start_latencies.txt")
	# os.system("head ./normal_vhive_bench/rnn/latencies/rps* -n 1 >> normal_rnn_cold_start_latencies.txt")

	plot_p99_percentile()
	plot_pull_images_and_snap_files()

if __name__ == "__main__":
	main()