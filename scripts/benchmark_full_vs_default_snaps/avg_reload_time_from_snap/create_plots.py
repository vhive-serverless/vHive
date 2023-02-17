# create bar plots to compare 3 sets of bidimensional data using matplotlib

import matplotlib.pyplot as plt
import numpy as np

# data collected by processing data manually through bash commands/oneliners or processing scripts
enhanced = {
    "helloworld": 193,
    "pyaes": 179,
    "rnn": 227,
}

default = {
    "helloworld": 92.790088,
    "pyaes": 89.389297,
    "rnn": 109.333548,
}

# create plot
fig, ax = plt.subplots()
index = np.arange(3)
bar_width = 0.35
opacity = 1

ax.yaxis.grid(True)
ax.set_axisbelow(True)

rects1 = plt.bar(index, default.values(), bar_width,
alpha=opacity,
color='b',
label='Default vHive')

rects2 = plt.bar(index + bar_width, enhanced.values(), bar_width,
alpha=opacity,
color='g',
label='Enhanced vHive')

plt.xlabel('Function type')
plt.ylabel('Time (ms)')
plt.title('Average reload time from snapshot')
plt.xticks(index + bar_width, [key.capitalize() for key in default.keys()])
plt.legend()

plt.tight_layout()

fig.savefig('cold_start_time.png', transparent=False)
