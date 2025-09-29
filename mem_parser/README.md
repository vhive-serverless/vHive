# Memory Parser - C Implementation

A high-performance, minimal footprint memory parser that extracts virtual-to-physical address mappings from Linux processes.

## Features

- **Extremely small footprint**: 14KB vs 1.5MB (Go version)
- **Zero-copy streaming**: Direct JSON output without buffering
- **Constant memory usage**: No memory growth with large processes
- **Robust error handling**: Graceful handling of EOF and edge cases
- **Static linking support**: For maximum portability
- **JSON output**: Compatible with the original Go version

## Build Options

### Dynamic linking (smallest)
```bash
gcc -O2 -Wall -Wextra -std=c99 -o mem_parser mem_parser.c
strip mem_parser
```

### Static linking (portable)
```bash
gcc -O2 -Wall -Wextra -std=c99 -static -o mem_parser_static mem_parser.c
strip mem_parser_static
```

### Using Makefile
```bash
make           # Dynamic linking
make static    # Static linking
make strip     # Strip symbols from existing binary
```

## Usage

```bash
./mem_parser <PID>
```

The program will create a JSON file named `pid_<PID>_pagemap.json` containing the memory mapping information.

## Size Comparison

| Version | Size | Notes |
|---------|------|-------|
| C (dynamic) | 14KB | Requires libc |
| C (static) | 882KB | Fully self-contained |
| Go (optimized) | 1.5MB | Includes runtime |

## Performance Improvements

- **108x smaller** than Go version (dynamic)
- **Streaming output**: No memory buffering required
- **Constant memory footprint**: Memory usage independent of process size
- Better error handling (no spurious EOF warnings)
- Faster startup time
- No garbage collection overhead

## Output Format

The output JSON format is identical to the original Go version:

```json
[
  {
    "virtual_address": 94123456789,
    "physical_address": 12345678901,
    "permissions": "r-xp"
  }
]
```