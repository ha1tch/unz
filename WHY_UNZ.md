# Why unz????

**unz** is a ZIP-compatible compressor that beats `zip -9` on source code and text files by 5-10% on average, while remaining compatible with the PKZIP format where possible.

### Motivation

Standard ZIP compression (DEFLATE) treats all files as streams of bytes. It doesn't know that `if err != nil` appears thousands of times in Go code, or that `def __init__(self):` is ubiquitous in Python. It rediscovers these patterns in every file, wasting bits.

**unz** tokenises source code using Byte Pair Encoding (BPE) before compression. Common language idioms become single tokens, and DEFLATE compresses the token stream instead of raw bytes. The vocabulary is shared (not embedded in each file), so there's minimal overhead.

The result: smaller archives for code, config files, and documentation—the files developers compress most often.

### Design Principles

- **Pure Go** — No external dependencies beyond the standard library. Builds anywhere Go builds.
- **PKZIP compatible** — Output files are valid ZIP archives. Standard tools can list contents; only extraction of BPE-compressed entries requires `unz`. Unz files *are* zip files, you only notice the difference if you use the improved Bpelate compression method. That's the reason we decided to use a different filename extension.
- **Automatic method selection** — The compressor tries multiple methods and picks the smallest result. You don't need to think about it.
- **Transparent fallback** — If BPE doesn't help (large files, binary data), it uses standard DEFLATE. You never get worse compression than `zip -9`.


