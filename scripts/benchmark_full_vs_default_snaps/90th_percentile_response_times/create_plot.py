import matplotlib.pyplot as plt
import matplotlib
import numpy as np

def avg(l: list):
    return sum(l) / len(l)

failover_enhanced_helloworld = avg([97984, 1099462, 19814, 20700]) / 1000
failover_enhanced_pyaes = avg([424681, 24522, 24218, 24348]) / 1000
failover_enhanced_rnn = avg([457758, 57967, 55676, 38089]) / 1000

failover_default_helloworld = avg([107956, 2109260, 21065, 20028]) / 1000
failover_default_pyaes = avg([153442, 22056, 1168301, 3168336]) / 1000
failover_default_rnn = avg([1614789, 59695, 1836472, 50409]) / 1000

default_cold_start = [i / 1000 for i in [1108646, 2153779, 1779823]]
enhanced_cold_start = [2135.0847999999996, 2170.1427999999996, 2160.18572]
failover_default = [failover_default_helloworld, failover_default_pyaes, failover_default_rnn]
failover_enhanced = [failover_enhanced_helloworld, failover_enhanced_pyaes, failover_enhanced_rnn]

# create grouped bar plot
fig, ax = plt.subplots()
fig, ax = plt.subplots(figsize=(12, 6))
ax.set_xlim(-0.5, 4)
index = np.arange(3)
bar_width = 0.2
opacity = 1

ax.yaxis.grid(True)
ax.set_axisbelow(True)  


rects1 = plt.bar(index, default_cold_start, bar_width,
alpha=opacity,
color='b',
label='Default vHive - first query')

rects2 = plt.bar(index + bar_width, enhanced_cold_start, bar_width,
alpha=opacity,
color='g',
label='Enhanced vHive - first query')

rects3 = plt.bar(index + bar_width * 2, failover_default, bar_width,
alpha=opacity,
color='darkblue',
label='Default vHive - subsequent queries')

rects4 = plt.bar(index + bar_width * 3, failover_enhanced, bar_width,
alpha=opacity,
color='darkgreen',
label='Enhanced vHive - subsequent queries')

plt.xlabel('Function type')
plt.ylabel('Time (ms)')
plt.title('90th precentile query response time')
plt.xticks(index + bar_width, ('Hello World', 'PyAES', 'RNN'))
plt.legend()
# plt.legend(loc='upper right', bbox_to_anchor=(1.4, 1),
#           ncol=1, fancybox=True)

# plt.tight_layout()
# plt.show()

fig.savefig('graph.png')

