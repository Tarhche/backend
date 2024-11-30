import {RolesUpsertForm} from "@/features/roles/components";
import {withPermissions} from "@/components/with-authorization";

function NewRolePage() {
  return <RolesUpsertForm />;
}

export default withPermissions(NewRolePage, {
  requiredPermissions: ["roles.create"],
});
