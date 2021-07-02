#include <iostream>
#include <thread>
#include <chrono>
#include <vector>
#include <list>
#include <ctime>
#include <tuple>
#include <cstring>
#include <cstdlib>

int parseLine(char* line){
    // This assumes that a digit will be found and the line ends in " Kb".
    int i = strlen(line);
    const char* p = line;
    while (*p <'0' || *p > '9') p++;
    line[i-3] = '\0';
    i = atoi(p);
    return i;
}

std::tuple<int, int> memoryConsumption() { //Note: this value is in KB!
    FILE* file = fopen("/proc/self/status", "r");
    int vm, rss = -1;
    char line[128];

    while (fgets(line, 128, file) != NULL){
        if (std::strncmp(line, "VmSize:", 7) == 0){
            vm = parseLine(line);
            continue;
        }
        if (strncmp(line, "VmRSS:", 6) == 0){
            rss = parseLine(line);
            continue;
        }
    }
    fclose(file);
    return std::make_tuple(vm, rss);
}

int main() {
    std::vector<char> buf(2048);
    std::list<char> v;

    for (;;) {
        std::cerr << "a" << std::endl;
    }

    try {
        for (;;) {
            v.push_back(rand() % 256);
        }
    } catch (const std::exception &e) {
        fprintf(stderr, "Exception");
        buf.resize(0);
        buf.shrink_to_fit();
        auto [vm, rss] = memoryConsumption();
        std::cerr << e.what() << "\n";
        std::cout << "VM: " << (vm / 1024.0f) << " mb, RSS: " << (rss / 1024.0f) << " mb" << std::endl;
        exit(1);
    }

    std::cout << "Successful exit" << std::endl;
    auto [vm, rss] = memoryConsumption();
    std::cout << "VM: " << (vm / 1024.0f) << " mb, RSS: " << (rss / 1024.0f) << " mb" << std::endl;
    return 1;
}
