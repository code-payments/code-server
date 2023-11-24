#include <vector>
#include <cstdint>
#include <cstring>

#include "kikcode_wrapper.h"
#include "kikcodes.h"

#define TOTAL_BYTE_COUNT    39
#define MAIN_BYTE_COUNT     35
#define DATA_BYTE_COUNT     22
#define PAYLOAD_BYTE_COUNT  20
#define ECC_BYTE_COUNT      13
#define ZERO_BYTES { 0 }

extern "C" {
  
char* kikcode_encode(const char* data, int dataSize) {
    std::vector<uint8_t> dataVector(data, data + dataSize);
    static unsigned char outData[MAIN_BYTE_COUNT] = ZERO_BYTES;

    char bytes[PAYLOAD_BYTE_COUNT] = ZERO_BYTES;
    memcpy(bytes, dataVector.data(), dataVector.size());

    kikCodeEncodeRemote(outData, (unsigned char *)bytes, 0);

    return (char*)outData;
}

char* kikcode_decode(const char* data, int dataSize) {
    std::vector<uint8_t> dataVector(data, data + dataSize);
    KikCodePayload payload;
    unsigned int type;
    unsigned int color;
    kikCodeDecode((unsigned char *)dataVector.data(), &type, &payload, &color);

    // Trim any tail zero bytes at the tail
    unsigned char *bytes = payload.group.invite_code;
    int length = sizeof(payload.group.invite_code);
    while (length > 0 && bytes[length - 1] == 0x0) {
        length--;
    }

    // Use a static buffer to store the result
    static unsigned char* output[MAIN_BYTE_COUNT] = {0};
    memcpy(output, bytes, length);

    return (char*)output;
}

} // extern "C"