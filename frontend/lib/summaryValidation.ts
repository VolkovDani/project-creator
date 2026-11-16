type SummaryContract = {
  version: number;
  lengthUnit: string;
  layaMaxBytes: number;
  required: boolean;
};

export function validateSummary(
  text: string,
  contract: SummaryContract,
): { valid: boolean; byteLength: number; layaEligible: boolean } {
  const byteLength = new TextEncoder().encode(text).length;
  const wellFormed = text.isWellFormed();
  const valid =
    contract.version === 2 &&
    contract.lengthUnit === 'utf8-bytes' &&
    Number.isSafeInteger(contract.layaMaxBytes) &&
    contract.layaMaxBytes > 0 &&
    wellFormed &&
    !text.includes('\0') &&
    (contract.required
      ? text.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '').length > 0
      : true);
  return {
    valid,
    byteLength,
    layaEligible: valid && byteLength <= contract.layaMaxBytes,
  };
}
