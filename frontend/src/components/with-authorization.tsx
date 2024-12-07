/**
 * The `withPermissions` HOC is designed to restrict access to pages based
 *  on user permissions.
 *
 * For conditional rendering of components based on user permissions,
 * consider using the `PermissionGuard` component located at
 * `src/components/permission-guard.tsx`.
 *
 * * This HOC is based on "PermissionGuard as well".
 */

import {Permissions} from "@/lib/app-permissions";
import {PermissionGuard} from "./permission-guard";
import {PermissionDeniedError} from "@/components/errors/dashboard-permission-denied";
import {Operator} from "@/lib/auth";

type Options = {
  requiredPermissions: Permissions[];
  operator?: Operator;
  fallback?: React.ReactNode;
};

export function withPermissions(
  Component: React.ComponentType<any>,
  options: Options,
) {
  const {requiredPermissions, operator, fallback} = options;

  const wrappedComponent = (props: any) => {
    return (
      <PermissionGuard
        allowedPermissions={requiredPermissions}
        operator={operator}
        fallback={fallback || <PermissionDeniedError />}
      >
        <Component {...props} />
      </PermissionGuard>
    );
  };

  return wrappedComponent;
}
