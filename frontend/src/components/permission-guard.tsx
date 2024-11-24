import {getUserPermissions} from "@/lib/auth";

type Props = {
  allowedPermissions: string[];
  children: React.ReactNode;
  fallback?: React.ReactNode;
};

export function PermissionGuard({
  allowedPermissions,
  children,
  fallback = null,
}: Props) {
  const userPermissions = getUserPermissions();

  const hasAccess = allowedPermissions.some((role) =>
    userPermissions.includes(role),
  );

  return hasAccess ? <>{children}</> : <>{fallback}</>;
}
