# create stacked bar plots to compare 3 functions using matplotlib

import matplotlib.pyplot as plt
import numpy as np

keys = ["helloworld", "pyaes", "rnn"]


patchfile = {
    "helloworld": 7.132226,
    "pyaes": 6.306645,
    "rnn": 10.846324,
}

infofile = {
    "helloworld": 4.241824,
    "pyaes": 4.135086,
    "rnn": 4.119246,
}

snapfile = {
    "helloworld": 4.252449,
    "pyaes": 4.120307,
    "rnn": 4.585438,
}

memfile = {
    "helloworld": 1552.02131,
    "pyaes": 1562.246895,
    "rnn": 2456.778129,
}

patchfile = [7.132226, 6.306645, 10.846324]
infofile = [4.241824, 4.135086, 4.119246]
snapfile = [4.252449, 4.120307, 4.585438]
memfile = [1552.02131, 1562.246895, 2456.778129]
pull = [s * 1000 for s in [3.19815, 1.90631, 12.39218]]

# patchfile = np.log10(patchfile)
# infofile = np.log10(infofile)
# snapfile = np.log10(snapfile)
# memfile = np.log10(memfile)
# pull = np.log10(pull)

# create plot
fig, ax = plt.subplots(figsize=(8, 6))
ax.set_xlim(-0.5, 3)
index = np.arange(3)
bar_width = 0.5
opacity = 1

# create stacked bar plot in logarithmic scale
# plt.yscale('log', base=10)

rects1 = plt.bar(index, patchfile, bar_width,
alpha=opacity,
color='b',
label='Patch file')

rects2 = plt.bar(index, infofile, bar_width,
alpha=opacity,
color='g',
label='Info file',
bottom=patchfile)

rects3 = plt.bar(index, snapfile, bar_width,
alpha=opacity,
color='r',
label='Snap file',
bottom=[i+j for i,j in zip(patchfile, infofile)])

rects5 = plt.bar(index, pull, bar_width,
alpha=opacity,
color='c',
label='Function image',
bottom=[i+j+k for i,j,k in zip(patchfile, infofile, snapfile)])

rects4 = plt.bar(index, memfile, bar_width,
alpha=opacity,
color='y',
label='Memory file',
bottom=[i+j+k+l for i,j,k,l in zip(patchfile, infofile, snapfile, pull)])

# rects4 = plt.bar(index, memfile, bar_width,
# alpha=opacity,
# color='y',
# label='Memory file',
# bottom=[i+j+k for i,j,k in zip(patchfile, infofile, snapfile)])

# rects5 = plt.bar(index, pull, bar_width,
# alpha=opacity,
# color='c',
# label='Pull image',
# bottom=[i+j+k+l for i,j,k,l in zip(patchfile, infofile, snapfile, memfile)])

ax.yaxis.grid(True)
ax.xaxis.grid(True)

plt.xlabel('Function')
plt.ylabel('Time (ms)')
plt.title('Preqrequisites latency for remote snapshots reloading')
plt.xticks(index, keys)
plt.legend()
# plt.legend(loc='upper right', bbox_to_anchor=(1.5, 1.05),
#           ncol=1, fancybox=True)

plt.tight_layout()
# plt.show()

# save plot
fig.savefig('graph.png')