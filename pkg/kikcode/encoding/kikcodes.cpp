#include "kikcodes.h"
#include "kikcode_encoding.h"

#include <cstring>
#include <iostream>

using namespace std;

int kikCodeEncodeUsername(
    unsigned char *out_data,
    const char *username,
    const unsigned int username_length,
    const unsigned short nonce,
    const unsigned int colour_code)
{
    UsernameKikCode kik_code(string(username, username_length), nonce, (KikCode::Colour)colour_code);

    kik_code.encode(out_data);

    return KIK_CODE_RESULT_SUCCESS;
}

int kikCodeEncodeGroup(
    unsigned char *out_data,
    const unsigned char *invite_code,
    const unsigned int colour_code)
{
    GroupKikCode kik_code(string((const char *)invite_code, 20), (KikCode::Colour)colour_code);

    kik_code.encode(out_data);

    return KIK_CODE_RESULT_SUCCESS;
}

int kikCodeEncodeRemote(
    unsigned char *out_data,
    const unsigned char *key,
    const unsigned int colour_code)
{
    RemoteKikCode kik_code(string((const char *)key, 20), (KikCode::Colour)colour_code);

    kik_code.encode(out_data);

    return KIK_CODE_RESULT_SUCCESS;
}

int kikCodeDecode(
    const unsigned char *data,
    unsigned int *out_type,
    KikCodePayload *out_payload,
    unsigned int *out_colour_code)
{
    KikCode *kik_code = KikCode::parse(data);

    if (!kik_code) {
        return KIK_CODE_RESULT_ERROR;
    }

    *out_type = (unsigned int)kik_code->type();
    *out_colour_code = (unsigned int)kik_code->colour();

    switch (kik_code->type()) {
    case KikCode::Type::Username: {
        UsernameKikCode *username_code = (UsernameKikCode *)kik_code;

        string username = username_code->username();

        memcpy(out_payload->username.username, username.c_str(), username.length());

        out_payload->username.username[username.length()] = '\0';

        out_payload->username.username_length = username.length();
        out_payload->username.nonce = username_code->nonce();

        break;
    }
    case KikCode::Type::Remote: {
        RemoteKikCode *remote_code = (RemoteKikCode *)kik_code;

        memcpy(out_payload->remote.payload, remote_code->payload().c_str(), 20);

        break;
    }
    case KikCode::Type::Group: {
        GroupKikCode *group_code = (GroupKikCode *)kik_code;

        memcpy(out_payload->group.invite_code, group_code->inviteCode().c_str(), 20);

        break;
    }
    }

    delete kik_code;

    return KIK_CODE_RESULT_SUCCESS;
}
