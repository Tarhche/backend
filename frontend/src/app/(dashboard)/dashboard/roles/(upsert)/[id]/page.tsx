import {notFound} from "next/navigation";
import {fetchRole} from "@/dal";
import {RolesUpsertForm} from "@/features/dashboard/roles-upsert-form";

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

export default UpdateRolePage;
