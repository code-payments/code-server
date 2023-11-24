#ifndef KIKCODES_WRAPPER_H
#define KIKCODES_WRAPPER_H

#include "stdlib.h"

#ifdef __cplusplus
extern "C" {
#endif // __cplusplus

char* kikcode_encode(const char* data, int dataSize);
char* kikcode_decode(const char* data, int dataSize);

#ifdef __cplusplus
}
#endif // __cplusplus

#endif // KIKCODES_WRAPPER_H