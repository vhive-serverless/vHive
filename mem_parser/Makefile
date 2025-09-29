CC = gcc
CFLAGS = -O2 -Wall -Wextra -std=c99
TARGET = mem_parser
SOURCE = mem_parser.c

$(TARGET): $(SOURCE)
	$(CC) $(CFLAGS) -o $(TARGET) $(SOURCE)

clean:
	rm -f $(TARGET)

strip: $(TARGET)
	strip $(TARGET)

static: $(SOURCE)
	$(CC) $(CFLAGS) -static -o $(TARGET) $(SOURCE)
	strip $(TARGET)

.PHONY: clean strip static