import sys
from PIL import Image

src, dst, width = sys.argv[1], sys.argv[2], int(sys.argv[3])
img = Image.open(src)
img.resize((width, int(img.height * width / img.width))).save(dst)
