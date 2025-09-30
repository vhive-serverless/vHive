#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <stdint.h>
#include <errno.h>

#define PAGE_SIZE 4096
#define MAX_LINE 512
#define MAX_PATH 256

typedef struct {
    uint64_t start_addr;
    uint64_t end_addr;
    char perms[5];
    uint64_t offset;
    char dev[6];
    uint64_t inode;
    char pathname[MAX_PATH];
} MemoryMapping;

static int parse_maps_file(const char *filepath, MemoryMapping **mappings, int *count) {
    FILE *file = fopen(filepath, "r");
    if (!file) {
        perror("fopen maps");
        return -1;
    }

    *mappings = NULL;
    *count = 0;
    int capacity = 64;
    *mappings = malloc(capacity * sizeof(MemoryMapping));
    if (!*mappings) {
        fclose(file);
        return -1;
    }

    char line[MAX_LINE];
    while (fgets(line, sizeof(line), file)) {
        if (*count >= capacity) {
            capacity *= 2;
            MemoryMapping *tmp = realloc(*mappings, capacity * sizeof(MemoryMapping));
            if (!tmp) {
                free(*mappings);
                fclose(file);
                return -1;
            }
            *mappings = tmp;
        }

        MemoryMapping *m = &(*mappings)[*count];
        char addr_range[32];
        
        if (sscanf(line, "%31s %4s %lx %5s %lu %255s",
                   addr_range, m->perms, &m->offset, m->dev, &m->inode, m->pathname) < 5) {
            continue;
        }

        char *dash = strchr(addr_range, '-');
        if (!dash) continue;
        *dash = '\0';

        m->start_addr = strtoull(addr_range, NULL, 16);
        m->end_addr = strtoull(dash + 1, NULL, 16);
        
        if (strlen(m->pathname) == 0) {
            strcpy(m->pathname, "[anonymous]");
        }

        (*count)++;
    }

    fclose(file);
    return 0;
}

static int process_and_output(const char *pagemap_path, MemoryMapping *mappings, int mapping_count, FILE *output) {
    FILE *pagemap = fopen(pagemap_path, "rb");
    if (!pagemap) {
        perror("fopen pagemap");
        return -1;
    }

    fprintf(output, "[\n");
    int page_count = 0;
    int first_page = 1;

    for (int i = 0; i < mapping_count; i++) {
        MemoryMapping *m = &mappings[i];
        
        for (uint64_t vaddr = m->start_addr; vaddr < m->end_addr; vaddr += PAGE_SIZE) {
            uint64_t offset = (vaddr / PAGE_SIZE) * 8;
            if (fseek(pagemap, offset, SEEK_SET) != 0) {
                continue;
            }

            uint64_t page_entry;
            if (fread(&page_entry, sizeof(page_entry), 1, pagemap) != 1) {
                continue;
            }

            // Check if page is present in RAM (bit 63)
            if ((page_entry >> 63) & 1) {
                uint64_t pfn = page_entry & ((1ULL << 55) - 1);
                if (pfn == 0) continue;

                uint64_t physical_addr = pfn * PAGE_SIZE + (vaddr % PAGE_SIZE);
                
                if (!first_page) {
                    fprintf(output, ",\n");
                }
                first_page = 0;
                
                fprintf(output, "  {\n");
                fprintf(output, "    \"virtual_address\": %lu,\n", vaddr);
                fprintf(output, "    \"physical_address\": %lu,\n", physical_addr);
                fprintf(output, "    \"permissions\": \"%.4s\",\n", m->perms);
                fprintf(output, "    \"pathname\": \"%s\",\n", m->pathname);
                fprintf(output, "    \"offset\": %lu\n", m->offset + (vaddr - m->start_addr));
                fprintf(output, "  }");

                page_count++;
            }
        }
    }

    fprintf(output, "\n]\n");
    fclose(pagemap);
    return page_count;
}

int main(int argc, char *argv[]) {
    if (argc < 2) {
        fprintf(stderr, "Usage: %s <PID>\n", argv[0]);
        return 1;
    }

    const char *pid = argv[1];
    char maps_file[64], pagemap_file[64], output_file[64];
    
    snprintf(maps_file, sizeof(maps_file), "/proc/%s/maps", pid);
    snprintf(pagemap_file, sizeof(pagemap_file), "/proc/%s/pagemap", pid);
    snprintf(output_file, sizeof(output_file), "pid_%s_pagemap.json", pid);

    MemoryMapping *mappings = NULL;
    int mapping_count = 0;
    
    if (parse_maps_file(maps_file, &mappings, &mapping_count) != 0) {
        fprintf(stderr, "Error parsing maps file\n");
        return 1;
    }

    FILE *output = fopen(output_file, "w");
    if (!output) {
        perror("fopen output");
        free(mappings);
        return 1;
    }
    
    int page_count = process_and_output(pagemap_file, mappings, mapping_count, output);
    fclose(output);
    
    if (page_count < 0) {
        fprintf(stderr, "Error processing page infos\n");
        free(mappings);
        return 1;
    }

    printf("Successfully wrote page map to %s (%d pages)\n", output_file, page_count);

    free(mappings);
    return 0;
}