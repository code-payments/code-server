#include <iomanip>
#include <stdexcept>
#include <cstring>

#include "kikcode_encoding.h"

#include "ReedSolomonEncoder.h"
#include "ReedSolomonDecoder.h"
#include "ReedSolomonException.h"
#include "IllegalArgumentException.h"
#include "IllegalStateException.h"

using namespace std;
using namespace zxing;

size_t KikCode::writeByte(unsigned char *out_data, size_t offset, unsigned char value)
{
    out_data[offset] = (uint8_t)(value & 0xff);

    return offset + 1;
}

size_t KikCode::writeShort(unsigned char *out_data, size_t offset, unsigned short value)
{
    out_data[offset]     = (uint8_t)(value & 0xff);
    out_data[offset + 1] = (uint8_t)((value & 0xff00) >> 8);

    return offset + 2;
}

size_t KikCode::writeInt(unsigned char *out_data, size_t offset, unsigned int value)
{
    out_data[offset]     = (uint8_t)(value & 0xff);
    out_data[offset + 1] = (uint8_t)((value & 0xff00) >> 8);
    out_data[offset + 2] = (uint8_t)((value & 0xff0000) >> 16);
    out_data[offset + 3] = (uint8_t)((value & 0xff000000) >> 24);

    return offset + 4;
}

size_t KikCode::writeLong(unsigned char *out_data, size_t offset, unsigned long long value)
{
    uint32_t high_dword = (uint32_t)(value >> 32);

    out_data[offset]     = (uint8_t)(value & 0xff);
    out_data[offset + 1] = (uint8_t)((value & 0xff00) >> 8);
    out_data[offset + 2] = (uint8_t)((value & 0xff0000) >> 16);
    out_data[offset + 3] = (uint8_t)((value & 0xff000000) >> 24);
    out_data[offset + 4] = (uint8_t)(high_dword & 0xff);
    out_data[offset + 5] = (uint8_t)((high_dword & 0xff00) >> 8);
    out_data[offset + 6] = (uint8_t)((high_dword & 0xff0000) >> 16);
    out_data[offset + 7] = (uint8_t)((high_dword & 0xff000000) >> 24);

    return offset + 8;
}

unsigned char KikCode::readByte(unsigned char *data, size_t offset)
{
    return data[offset];
}

unsigned short KikCode::readShort(unsigned char *data, size_t offset)
{
    return data[offset] | (data[offset + 1] << 8);
}

unsigned int KikCode::readInt(unsigned char *data, size_t offset)
{
    return data[offset] | (data[offset + 1] << 8) | (data[offset + 2] << 16) | (data[offset + 3] << 24);
}

unsigned long long KikCode::readLong(unsigned char *data, size_t offset)
{
    unsigned int low = data[offset] | (data[offset + 1] << 8) | (data[offset + 2] << 16) | (data[offset + 3] << 24);
    unsigned int high = data[offset + 4] | (data[offset + 5] << 8) | (data[offset + 6] << 16) | (data[offset + 7] << 24);

    return (unsigned long long)low | ((unsigned long long)high << 32);
}

void KikCode::decode(uint8_t *data)
{
    // type
    type_ = (KikCode::Type)(data[0] & 0x1f);

    // colour code
    int colour = (data[0] & 0xe0) >> 5;
    colour |= (data[1] & 0x07) << 3;

    colour_ = (KikCode::Colour)colour;

    // extra
    extra_ = (data[1] & 0xf8) >> 3;
}

KikCode::KikCode(KikCode::Type type, KikCode::Colour colour)
: type_(type)
, colour_(colour)
, extra_(0)
{
}

KikCode::~KikCode()
{
}

KikCode::Colour KikCode::colour() const
{
    return colour_;
}

KikCode::Type KikCode::type() const
{
    return type_;
}

uint8_t KikCode::extra() const
{
    return extra_;
}

KikCode *KikCode::parse(const uint8_t *data)
{
    uint8_t data_section[KIK_CODE_ALL_BYTE_COUNT];

    ArrayRef<int> codeword_ints(KIK_CODE_ALL_BYTE_COUNT);

    unsigned char reordered_data[KIK_CODE_ALL_BYTE_COUNT];

    // put the ECC back on the end
    for (size_t i = 0; i < KIK_CODE_ECC_BYTE_COUNT; ++i) {
        reordered_data[i + KIK_CODE_DATA_BYTE_COUNT] = data[i];
    }

    for (size_t i = KIK_CODE_ECC_BYTE_COUNT; i < KIK_CODE_ALL_BYTE_COUNT; ++i) {
        reordered_data[i - KIK_CODE_ECC_BYTE_COUNT] = data[i];
    }

    for (size_t i = 0; i < KIK_CODE_ALL_BYTE_COUNT; ++i) {
        codeword_ints[i] = reordered_data[i] & 0xff;
    }

    ReedSolomonDecoder rs_decoder(GenericGF::QR_CODE_FIELD_256);

    try {
        rs_decoder.decode(codeword_ints, KIK_CODE_ECC_BYTE_COUNT-1);
    }
    catch (ReedSolomonException const &ignored) {
        (void)ignored;
        return nullptr;
    }
    catch (IllegalArgumentException const &ignored) {
        (void)ignored;
        return nullptr;
    }
    catch (IllegalStateException const &ignored) {
        (void)ignored;
        return nullptr;
    }

    for (size_t i = 0; i < KIK_CODE_ALL_BYTE_COUNT; ++i) {
        data_section[i] = (uint8_t)codeword_ints[i];
    }

    // type
    uint8_t type = data_section[0] & 0x1f; // lower 5 bits

    // colour code
    uint8_t colour_code = (data_section[0] & 0xe0) >> 5; // upper 3 bits

    colour_code |= (data_section[1] & 0x07) << 5; // lower 3 bits

    KikCode *kik_code_result = nullptr;

    switch (type) {
    case 1:
        kik_code_result = new UsernameKikCode((Colour)colour_code);
        break;
    case 2:
        kik_code_result = new RemoteKikCode((Colour)colour_code);
        break;
    case 3:
        kik_code_result = new GroupKikCode((Colour)colour_code);
        break;
    }

    if (kik_code_result) {
        kik_code_result->decode(data_section);
    }

    return kik_code_result;
}

void KikCode::encode(uint8_t *out_data)
{
    // type
    out_data[0] = (uint32_t)type() & 0x1f; // lower 5 bits

    // colour code
    out_data[0] |= ((uint32_t)colour() << 5) & 0xe0; // upper 3 bits
    out_data[1]  = ((uint32_t)colour() >> 3) & 0x07; // lower 3 bits

    // extra
    out_data[1] |= ((uint32_t)extra() << 3) & 0xf8; // upper 5 bits

    ArrayRef<int> codeword_ints(KIK_CODE_ALL_BYTE_COUNT);

    for (size_t i = 0; i < KIK_CODE_DATA_BYTE_COUNT; ++i) {
        codeword_ints[i] = out_data[i] & 0xff;
    }

    // apply error correction
    ReedSolomonEncoder rs_encoder(*GenericGF::QR_CODE_FIELD_256);

    try {
        rs_encoder.encode(codeword_ints, KIK_CODE_ECC_BYTE_COUNT - 1);
    }
    catch (ReedSolomonException const &ignored) {
        (void)ignored;
        throw invalid_argument("Error correction error");
    }
    catch (IllegalArgumentException const &ignored) {
        (void)ignored;
        throw invalid_argument("Error correction error");
    }
    catch (IllegalStateException const &ignored) {
        (void)ignored;
        throw invalid_argument("Error correction error");
    }

    // move the ECC to the front of the data
    for (size_t i = 0; i < KIK_CODE_ECC_BYTE_COUNT; ++i) {
        out_data[i] = (uint8_t)codeword_ints[i + KIK_CODE_DATA_BYTE_COUNT];
    }

    for (size_t i = 0; i < KIK_CODE_DATA_BYTE_COUNT; ++i) {
        out_data[i + KIK_CODE_ECC_BYTE_COUNT] = (uint8_t)codeword_ints[i];
    }
}

UsernameKikCode::UsernameKikCode(Colour colour)
: KikCode(KikCode::Type::Username, colour)
{
}

void UsernameKikCode::decode(uint8_t *data_section)
{
    KikCode::decode(data_section);

    char username[32];
    size_t username_length = extra() + 2;

    if (username_length > 24) {
        throw invalid_argument("Username too long");
    }

    // See https://github.com/kikinteractive/kik-product/wiki/Scan-Code#username
    // for codepoints and 6-bit encoding
    for (size_t i = 0; i < username_length; ++i) {
        // offset is computed as the position in the 6-bit encoded input bytes
        // based on the position in the 8-bit destination

        // the username starts 2 bytes from the front of the data section so always add 2.
        // For every character in the destination username, we are effectively advancing 3/4
        // positions in the input bytes, we can't do this because you can't move to intermediate
        // bit positions so we find the start of the nearest 3-byte sequence and then use (i % 4)
        // to determine which position in the 6-bit encoding we are looking at currently
        int offset = (i - (i % 4)) * 3 / 4 + 2;
        int codepoint = 0;
        char ch = '\0';

        // every 4 characters in the username maps to a sequence of 3 packed bytes
        // in the 6-bit encoding
        //     0 1 2 3 4 5 6 7  0 1 2 3 4 5 6 7  0 1 2 3 4 5 6 7
        // 0 - 1 1 1 1 1 1 0 0  0 0 0 0 0 0 0 0  0 0 0 0 0 0 0 0 = 0x3f
        // 1 - 0 0 0 0 0 0 1 1  1 1 1 1 0 0 0 0  0 0 0 0 0 0 0 0 = 0xc0 >> 6 | 0x0f << 2
        // 2 - 0 0 0 0 0 0 0 0  0 0 0 0 1 1 1 1  1 1 0 0 0 0 0 0 = 0xf0 >> 4 | 0x03 << 4
        // 3 - 0 0 0 0 0 0 0 0  0 0 0 0 0 0 0 0  0 0 1 1 1 1 1 1 = 0xfc >> 2

        switch (i % 4) {
        case 0:
            codepoint = (data_section[offset] & 0x3f);
            break;
        case 1:
            codepoint = ((data_section[offset] & 0xc0) >> 6) | ((data_section[offset + 1] & 0x0f) << 2);
            break;
        case 2:
            codepoint = ((data_section[offset + 1] & 0xf0) >> 4) | ((data_section[offset + 2] & 0x03) << 4);
            break;
        case 3:
            codepoint = (data_section[offset + 2] & 0xfc) >> 2;
            break;
        }

        if (codepoint < 26) {
            ch = 'A' + codepoint;
        }
        else if (codepoint < 52) {
            ch = 'a' + (codepoint - 26);
        }
        else if (codepoint < 62) {
            ch = '0' + (codepoint - 52);
        }
        else if (codepoint == 62) {
            ch = '.';
        }
        else if (codepoint == 63) {
            ch = '_';
        }
        else {
            throw invalid_argument("Unknown codepoint");
        }

        username[i] = ch;
    }

    // null-terminate the username
    username[username_length] = '\0';

    username_ = std::string(username, username_length);
    nonce_ = data_section[20] | ((int)data_section[21] << 8);
}

UsernameKikCode::UsernameKikCode(const std::string &username, uint16_t nonce, Colour colour)
: KikCode(KikCode::Type::Username, colour)
, username_(username)
, nonce_(nonce)
{
}

void UsernameKikCode::encode(uint8_t *out_data)
{
    memset(out_data, 0, KIK_CODE_DATA_BYTE_COUNT);

    extra_ = username_.length() - 2;

    // encode 6-bit username section
    size_t i = 0;
    if (username_.length() < 2) {
        throw invalid_argument("Username too short");
    }
    else if (username_.length() > 24) {
        throw invalid_argument("Username too long");
    }

    int offset = 0;

    // See UsernameKikCode::decode for encoding reference
    for (size_t l = username_.length(); i < l; ++i) {
        offset = (i - (i % 4)) * 3 / 4 + 2;

        char ch = username_[i];
        int value = 0;

        if (ch >= 'A' && ch <= 'Z') {
            value = (ch - 'A');
        }
        else if (ch >= 'a' && ch <= 'z') {
            value = (ch - 'a') + 26;
        }
        else if (ch >= '0' && ch <= '9') {
            value = (ch - '0') + 52;
        }
        else if (ch == '.') {
            value = 62;
        }
        else if (ch == '_') {
            value = 63;
        }
        else {
            throw invalid_argument("Invalid character");
            return;
        }

        switch (i % 4) {
        case 0:
            out_data[offset] = (value & 0x3f);
            break;
        case 1:
            out_data[offset]  |= (value & 0x03) << 6;
            out_data[offset+1] = (value & 0x3c) >> 2;
            break;
        case 2:
            out_data[offset+1] |= (value & 0x0f) << 4;
            out_data[offset+2]  = (value & 0x30) >> 4;
            break;
        case 3:
            out_data[offset+2] |= (value & 0x3f) << 2;
            break;
        }
    }

    for (offset += 3; offset < 20; ++offset) {
        out_data[offset] = 0xaa ^ offset;
    }

    writeShort(out_data, 20, nonce_);

    // finish the encoding and write the error correction
    KikCode::encode(out_data);
}

string UsernameKikCode::username() const
{
    return username_;
}

uint16_t UsernameKikCode::nonce() const
{
    return nonce_;
}

RemoteKikCode::RemoteKikCode(Colour colour)
: KikCode(KikCode::Type::Remote, colour)
{
}

RemoteKikCode::RemoteKikCode(const std::string &payload, Colour colour)
: KikCode(KikCode::Type::Remote, colour)
, payload_(payload)
{
}

void RemoteKikCode::decode(uint8_t *data_section)
{
    KikCode::decode(data_section);

    payload_ = string(reinterpret_cast<char *>(data_section + 2), KIK_CODE_PAYLOAD_BYTE_COUNT);
}

void RemoteKikCode::encode(uint8_t *out_data)
{
    memcpy(out_data + 2, payload_.c_str(), KIK_CODE_PAYLOAD_BYTE_COUNT);

    // finish the encoding and write the error correction
    KikCode::encode(out_data);
}

std::string RemoteKikCode::payload() const
{
    return payload_;
}

GroupKikCode::GroupKikCode(Colour colour)
: KikCode(KikCode::Type::Group, colour)
{
}

GroupKikCode::GroupKikCode(const std::string &invite_code, Colour colour)
: KikCode(KikCode::Type::Group, colour)
, invite_code_(invite_code)
{
}

void GroupKikCode::decode(uint8_t *data_section)
{
    KikCode::decode(data_section);

    invite_code_ = string(reinterpret_cast<char *>(data_section + 2), KIK_CODE_PAYLOAD_BYTE_COUNT);
}

void GroupKikCode::encode(uint8_t *out_data)
{
    memcpy(out_data + 2, invite_code_.c_str(), KIK_CODE_PAYLOAD_BYTE_COUNT);

    // finish the encoding and write the error correction
    KikCode::encode(out_data);
}

std::string GroupKikCode::inviteCode() const
{
    return invite_code_;
}
