import type { FieldError } from './validation';

type StringRule = { required: boolean; maxLength: number };

type RequirementContract = {
  version: number;
  lengthUnit: string;
  trim: string;
  fields: { text: StringRule; changeReason: StringRule & { blank: string } };
};

function validateContract(c: RequirementContract) {
  if (
    !c ||
    Object.keys(c).sort().join() !== 'fields,lengthUnit,trim,version' ||
    c.version !== 1 ||
    c.lengthUnit !== 'unicode-code-points' ||
    c.trim !== 'ascii-space-tab-lf-cr' ||
    Object.keys(c.fields).sort().join() !== 'changeReason,text' ||
    Object.keys(c.fields.text).sort().join() !== 'maxLength,required' ||
    Object.keys(c.fields.changeReason).sort().join() !== 'blank,maxLength,required' ||
    c.fields.changeReason.blank !== 'unicode-white-space' ||
    Object.values(c.fields).some(
      (rule) =>
        rule.required !== true || !Number.isSafeInteger(rule.maxLength) || rule.maxLength <= 0,
    )
  )
    throw Error('Unsupported requirement contract');
}

function fieldErrors(
  field: keyof RequirementContract['fields'],
  value: string,
  blank: boolean,
  rule: StringRule,
): FieldError[] {
  const path = ['input', field];
  if (!value.isWellFormed())
    return [{ path, code: 'INVALID_UTF8', messageKey: 'validation.invalidEncoding', params: {} }];
  if (blank) return [{ path, code: 'REQUIRED', messageKey: 'validation.required', params: {} }];
  if (Array.from(value).length > rule.maxLength)
    return [
      {
        path,
        code: 'MAX_LENGTH',
        messageKey: 'validation.maxLength',
        params: { max: rule.maxLength },
      },
    ];
  return [];
}

export function validateRequirement(c: RequirementContract, input: string) {
  validateContract(c);
  const normalized = input.replace(/^[ \t\r\n]+|[ \t\r\n]+$/g, '');
  return { normalized, errors: fieldErrors('text', normalized, !normalized, c.fields.text) };
}

export function validateChangeReason(c: RequirementContract, input?: string | null) {
  validateContract(c);
  const value = input ?? '';
  return fieldErrors(
    'changeReason',
    value,
    /^\p{White_Space}*$/u.test(value),
    c.fields.changeReason,
  );
}
