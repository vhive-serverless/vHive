# MIT License
#
# Copyright (c) 2021 Michal Baczun and EASE lab
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

import unittest
import storage
import random


class MyTest(unittest.TestCase):
    def test_s3(self):
        s = storage.Storage("S3","storage-module-test")
        msg = bytes(random.randint(1,10000))
        s.put("aws-test-key", msg)
        self.assertEqual(s.get("aws-test-key"), msg)

    def test_elasticache(self):
        s = storage.Storage("ELASTICACHE","redis://test2.0vgvbw.ng.0001.usw1.cache.amazonaws.com:6379")
        self.assertEqual(s.elasticache_client.ping(), True)
        msg = b"test msg"
        s.put("elasticache-test-key", msg)
        self.assertEqual(s.get("elasticache-test-key"), msg)