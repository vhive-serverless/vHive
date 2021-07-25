import imutils
from imutils.video import FileVideoStream
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

    vs = FileVideoStream(temp.name).start()
    for i in range(int(args.n)):
        frame = vs.read()
        image_bytes = cv2.imencode('.jpg', frame)[1].tobytes()

    end_time = now()
    vs.stop()
    total_time = ( end_time - begin_time ) * 1000
    times.append(total_time)

temp.close()

tot = 0
for i in times:
    tot += i
tot /= 5
print("Avg taken: %dms" % tot)