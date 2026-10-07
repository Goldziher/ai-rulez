package workspace

import (
	"bytes"
	"io/fs"
	"strconv"
	"strings"

	"github.com/samber/oops"

	"github.com/Goldziher/ai-rulez/v5/internal/gitutil"
)

// batchChunkBytes bounds the file contents one `git cat-file --batch` run returns.
const batchChunkBytes = 16 << 20

// BatchReader is implemented by a snapshot that can read many files with few
// git processes instead of one per file.
type BatchReader interface {
	// ReadBatch calls yield with the content of each named regular file, in the
	// order given. Contents are not kept by the snapshot.
	ReadBatch(names []string, yield func(name string, data []byte) error) error
}

// ReadBatch implements BatchReader.
func (s *snapshot) ReadBatch(names []string, yield func(string, []byte) error) error {
	for start := 0; start < len(names); {
		end, size := start, int64(0)
		for end < len(names) {
			n, ok := s.nodes[names[end]]
			if !ok || n.mode.IsDir() || n.mode&fs.ModeSymlink != 0 {
				return oops.With("path", names[end]).Errorf("%s is not a regular file of the snapshot", names[end])
			}
			if end > start && size+n.size > batchChunkBytes {
				break
			}
			size += n.size
			end++
		}
		if err := s.readChunk(names[start:end], size, yield); err != nil {
			return err
		}
		start = end
	}
	return nil
}

func (s *snapshot) readChunk(names []string, size int64, yield func(string, []byte) error) error {
	var in bytes.Buffer
	for _, name := range names {
		in.WriteString(s.nodes[name].oid + "\n")
	}
	res := s.git.ExecStdin(s.ctx, s.dir, in.Bytes(), size+int64(len(names))*128+4096, "cat-file", "--batch")
	if err := gitutil.ResultErr(res); err != nil {
		return oops.With("commit", s.commit).Wrapf(err, "read objects")
	}
	if res.StdoutTruncated {
		return oops.With("commit", s.commit).Errorf("objects are larger than the read cap")
	}
	out := res.Stdout
	for _, name := range names {
		n := s.nodes[name]
		nl := bytes.IndexByte(out, '\n')
		if nl < 0 {
			return oops.With("path", name).Errorf("truncated cat-file answer for %s", name)
		}
		fields := strings.Fields(string(out[:nl]))
		if len(fields) != 3 || fields[0] != n.oid || fields[1] != "blob" {
			return oops.With("path", name).Errorf("unexpected cat-file answer %q for %s", out[:nl], name)
		}
		length, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || length != n.size || int64(len(out)) < int64(nl)+1+length+1 {
			return oops.With("path", name).Errorf("cat-file reports %s bytes for %s, ls-tree %d", fields[2], name, n.size)
		}
		body := out[nl+1 : nl+1+int(length)]
		out = out[nl+1+int(length)+1:]
		if err := yield(name, body); err != nil {
			return err
		}
	}
	return nil
}
