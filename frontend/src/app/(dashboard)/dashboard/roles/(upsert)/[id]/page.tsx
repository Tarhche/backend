import {notFound} from "next/navigation";
import {RolesUpsertForm} from "@/features/roles/components/roles-upsert-form";
import {fetchRole} from "@/dal/private/roles";
import {withPermissions} from "@/components/with-authorization";

type Props = {
  params: {
    id?: string;
  };
};

async function UpdateRolePage({params}: Props) {
  const roleId = params.id;
  if (roleId === undefined) {
    notFound();
  }
  const role = await fetchRole(roleId);

  return (
    <RolesUpsertForm
      defaultValues={{
        roleId: role.uuid,
        defaultRoleName: role.name,
        defaultRoleDescription: role.description,
        defaultUsers: role.user_uuids,
        defaultPermissions: role.permissions,
      }}
    />
  );
}

export default withPermissions(UpdateRolePage, {
  requiredPermissions: ["roles.show", "roles.update"],
  operator: "AND",
});
