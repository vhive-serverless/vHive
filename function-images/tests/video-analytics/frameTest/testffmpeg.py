import ffmpeg
import tempfile
import io
import argparse

from timeit import default_timer as now

parser = argparse.ArgumentParser()
parser.add_argument("-n", "--n", dest = "n", default = "1", help="number of frames")
args = parser.parse_args()

vid = open('video.mp4', 'rb')

begin_tempfile = now()
temp = tempfile.NamedTemporaryFile(suffix=".mp4", dir=".")
temp.write(vid.read())
temp.seek(0)
end_tempfile_write = now()
write_time = (end_tempfile_write - begin_tempfile )*1000
print("tempfile write time: %dms" % write_time)

vid.close()

times = []
for j in range(5):
    begin_time = now()
    for i in range(int(args.n)):
        out, _ = (
            ffmpeg
            .input(temp.name)
            .filter('select', 'gte(n,{})'.format(i+1))
            .output('pipe:', vframes=1, format='image2', vcodec='mjpeg')
            .run(capture_stdout=True)
        )

    end_time = now()
    total_time = ( end_time - begin_time ) * 1000
    times.append(total_time)

temp.close()



tot = 0
for i in times:
    tot += i
tot /= 5
print("Avg taken: %dms" % tot)