import sys, time, urllib.request

deadline = time.time() + 120
while True:
    try:
        urllib.request.urlopen(sys.argv[1], timeout=2)
        break
    except OSError:
        if time.time() > deadline:
            sys.exit(1)
        time.sleep(2)
