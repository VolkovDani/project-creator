type SummaryContract = { version: number; lengthUnit: string; maxBytes: number; required: boolean };

export function validateSummary(
  text: string,
  contract: SummaryContract,
): { valid: boolean; byteLength: number } {
  const byteLength = new TextEncoder().encode(text).length;
  const wellFormed = text.isWellFormed();
  return {
    valid:
      contract.version === 1 &&
      contract.lengthUnit === 'utf8-bytes' &&
      wellFormed &&
      (contract.required
        ? text.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '').length > 0
        : true) &&
      byteLength <= contract.maxBytes,
    byteLength,
  };
}
