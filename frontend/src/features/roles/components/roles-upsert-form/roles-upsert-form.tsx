import {PermissionList} from "./permissions-list";
import {FormFields} from "./form-fields";
import {DefaultValues} from "./form-fields";

type Props = {
  defaultValues: DefaultValues & {
    defaultPermissions: string[];
  };
};

export function RolesUpsertForm({defaultValues}: Partial<Props>) {
  return (
    <FormFields defaultValues={defaultValues}>
      <PermissionList defaultPermissions={defaultValues?.defaultPermissions} />
    </FormFields>
  );
}
