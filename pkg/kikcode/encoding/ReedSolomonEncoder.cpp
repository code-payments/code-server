#include <iostream>

#include <memory>
#include "ReedSolomonEncoder.h"
#include "ReedSolomonException.h"
#include "IllegalArgumentException.h"
#include "IllegalStateException.h"

using std::vector;
using zxing::Ref;
using zxing::ArrayRef;
using zxing::ReedSolomonEncoder;
using zxing::GenericGFPoly;
using zxing::IllegalStateException;

// VC++
using zxing::GenericGF;

ReedSolomonEncoder::ReedSolomonEncoder(GenericGF &field) : field_(field)
{
        ArrayRef<int> initializeArray(new Array<int> (2));

        initializeArray[0] = 1;
        initializeArray[1] = 1;

    cachedGenerators_.push_back(Ref<GenericGFPoly>(new GenericGFPoly(field_, initializeArray)));
  }

  Ref<GenericGFPoly> ReedSolomonEncoder::buildGenerator(int degree)
  {
    if (degree >= (int)cachedGenerators_.size())
    {
      Ref<GenericGFPoly> lastGenerator = cachedGenerators_[cachedGenerators_.size() - 1];
      for (int d = cachedGenerators_.size(); d <= degree; d++) {
        ArrayRef<int> initializeArray(new Array<int> (2));
        initializeArray[0] = 1;
        initializeArray[1] = field_.exp(d - 1);
        Ref<GenericGFPoly> poly(new GenericGFPoly(field_, initializeArray));
        Ref<GenericGFPoly> nextGenerator = lastGenerator->multiply(poly);
        cachedGenerators_.push_back(nextGenerator);
        lastGenerator = nextGenerator;
      }
    }
    return cachedGenerators_[degree];
  }

  void ReedSolomonEncoder::encode(ArrayRef<int> toEncode, int ecBytes)
  {
    if (ecBytes == 0)
    {
      throw new IllegalArgumentException("No error correction bytes");
    }
    int dataBytes = toEncode->size() - ecBytes;
    if (dataBytes <= 0)
    {
      throw new IllegalArgumentException("No data bytes provided");
    }
    Ref<GenericGFPoly> generator = buildGenerator(ecBytes);
    ArrayRef<int> infoCoefficients(new Array<int>(dataBytes));

    for (int i = 0; i < dataBytes; ++i) {
        infoCoefficients[i] = toEncode[i];
    }

    //System.arraycopy(toEncode, 0, infoCoefficients, 0, dataBytes);
    Ref<GenericGFPoly> info(new GenericGFPoly(field_, infoCoefficients));
    info = info->multiplyByMonomial(ecBytes, 1);


    Ref<GenericGFPoly> remainder = info->divide(generator)[1];
    ArrayRef<int> coefficients = remainder->getCoefficients();
    int numZeroCoefficients = ecBytes - coefficients->size();

    for (int i = 0; i < numZeroCoefficients; i++) {
        toEncode[dataBytes + i] = 0;
    }

    for (int i = 0; i < coefficients->size(); ++i) {
        toEncode[dataBytes + numZeroCoefficients + i] = coefficients[i];
    }
  }
