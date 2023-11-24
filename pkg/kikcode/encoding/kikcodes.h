#ifndef __KIKCODES_H__
#define __KIKCODES_H__

#define KIK_CODE_RESULT_SUCCESS 0
#define KIK_CODE_RESULT_ERROR   1

extern "C" {

    union KikCodePayload {
        struct {
            char username[32];
            unsigned int username_length;
            unsigned short nonce;
        } username;

        struct {
            unsigned char invite_code[20];
        } group;

        struct {
            unsigned char payload[20];
        } remote;
    };

    // username codes
    int kikCodeEncodeUsername(
        unsigned char *out_data,
        const char *username,
        const unsigned int username_length,
        const unsigned short nonce,
        const unsigned int colour_code);

    // group codes
    int kikCodeEncodeGroup(
        unsigned char *out_data,
        const unsigned char *invite_code,
        const unsigned int colour_code);

    // remote codes
    int kikCodeEncodeRemote(
        unsigned char *out_data,
        const unsigned char *key,
        const unsigned int colour_code);

    int kikCodeDecode(
        const unsigned char *data,
        unsigned int *out_type,
        KikCodePayload *out_payload,
        unsigned int *out_colour_code);

    }

#endif // __KIKCODES_H__
