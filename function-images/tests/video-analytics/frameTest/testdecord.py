import decord
import cv2
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

    vr = decord.VideoReader('video.mp4', ctx=decord.cpu(0))
    for i in range(int(args.n)):
        frame = vr[i].asnumpy()
        image_bytes = cv2.imencode('.jpg', frame)[1].tobytes()

    end_time = now()
    total_time = ( end_time - begin_time ) * 1000
    times.append(total_time)

temp.close()

tot = 0
for i in times:
    tot += i
tot /= 5
print("Avg taken: %dms" % tot)