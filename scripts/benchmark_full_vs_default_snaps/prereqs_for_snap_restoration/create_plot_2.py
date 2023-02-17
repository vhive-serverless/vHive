# create stacked bar plots to compare 3 functions using matplotlib

import matplotlib.pyplot as plt
import matplotlib
import numpy as np

hello = {
    'patchfile': 7.132226,
    'infofile': 4.241824,
    'snapfile': 4.252449,
    'memfile': 1552.02131,
    'function image': 3198.15,
}
pyaes = {
    'patchfile': 6.306645,
    'infofile': 4.135086,
    'snapfile': 4.120307,
    'memfile': 1562.246895,
    'function image': 1906.31,
}
rnn = {
    'patchfile': 10.846324,
    'infofile': 4.119246,
    'snapfile': 4.585438,
    'memfile': 2456.778129,
    'function image': 12392.18,
}

symbols = [4, 5, '.', '1', '2', '3', '+', '_', 'x', 'o', 's', 'd', 'v', '*', '<', '>', 'p', 'h', 'H', 'D', '8']
subplots = {
    'helloword': (symbols[0], hello),
    'pyaes': (symbols[1], pyaes),
    'rnn': (symbols[2], rnn),
}

# create plot
fig, ax = plt.subplots()
index = np.arange(3)
bar_width = 0.35
opacity = 1

x = ['patchfile', 'infofile', 'snapfile', 'memfile', 'function image']
# create graph plot in logarithmic scale base 16

plt.xlabel('')
plt.ylabel('Time (ms)')
plt.title('Prerequisite latency for remote snapshots reloading')
plt.xticks(range(len(x)), x)
plt.yscale('log', base=10)

ax.yaxis.grid(True)
ax.xaxis.grid(True)

for name, (symbol, sub) in subplots.items():
    y = [sub['patchfile'], sub['infofile'], sub['snapfile'], sub['memfile'], sub['function image']]
    #plot using points, not lines
    plt.plot(y, marker=symbol, markersize=10, linestyle="None", label=name)
    # plt.plot(y, label=name)

# create legend
plt.legend(title='Function type')

plt.tight_layout()
# plt.show()

# save plot
fig.savefig('graph2.png', transparent=False)