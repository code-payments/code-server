#ifndef __REED_SOLOMON_ENCODER_H__
#define __REED_SOLOMON_ENCODER_H__

#include <memory>
#include <vector>
#include "Counted.h"
#include "Array.h"
#include "GenericGFPoly.h"
#include "GenericGF.h"

namespace zxing {
    class ReedSolomonEncoder {
    private:
        GenericGF &field_;
        std::vector<Ref<GenericGFPoly> > cachedGenerators_;
        
        Ref<GenericGFPoly> buildGenerator(int degree);
    
    public:
        ReedSolomonEncoder(GenericGF &field);
        void encode(ArrayRef<int> toEncode, int ecBytes);
    };
}

#endif // __REED_SOLOMON_ENCODER_H__
