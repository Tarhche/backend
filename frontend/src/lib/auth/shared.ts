export type Operator = "AND" | "OR";

export function hasPermission(
  userPermissions: string[],
  allowedPermissions: string[],
  operator: Operator = "OR",
) {
  if (allowedPermissions.length === 0) {
    return true;
  }
  if (operator === "AND") {
    return allowedPermissions.every((permission) =>
      userPermissions.includes(permission),
    );
  } else if (operator === "OR") {
    return allowedPermissions.some((permission) =>
      userPermissions.includes(permission),
    );
  }

  throw new Error(`unsupported operator ${operator}`);
}
