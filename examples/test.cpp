#include <iostream>
#include <thread>
#include <chrono>
#include <vector>
#include <ctime>
#
int main() {
    std::vector<int> v;

    for (int i = 0; i < 3; i++) {
        for (int j = 0; j < 100000; j++) {
            v.push_back((int)std::time(nullptr));
        }
        std::cerr << i + 1 << std::endl;
        std::this_thread::sleep_for(std::chrono::milliseconds(600));
    }
    std::cout << "Hello\", world!" << std::endl;
    return (v.size() + v.back()) % 60;
}
