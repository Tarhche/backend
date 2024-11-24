export function hasPermission(
  userPermissions: string[],
  allowedPermissions: string[],
) {
  return allowedPermissions.some((permission) =>
    userPermissions.includes(permission),
  );
}
