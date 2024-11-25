/**
 * This type recursively extracts all string values from a
 * Record<string, Record<string, ...>> structure.
 */
export type ExtractStrings<T> = T extends string
  ? T
  : T extends object
    ? {[K in keyof T]: ExtractStrings<T[K]>}[keyof T]
    : never;
