"use client";
import {useFormState} from "react-dom";
import {Group, Stack, Textarea} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {updateSettingAction} from "../actions/update-setting";

type Props = {
  config: {
    userDefaultRoles: string[];
  };
};

export function AppSettingForm({config}: Props) {
  const [state, dispatch] = useFormState(updateSettingAction, {success: false});

  return (
    <form action={dispatch}>
      <Stack>
        <Textarea
          name="user_default_roles"
          label="نقش پیشفرض کاربران"
          rows={4}
          defaultValue={config.userDefaultRoles}
          dir="ltr"
          styles={{
            input: {
              textAlign: "left",
            },
          }}
          error={state.fieldErrors?.user_default_roles || ""}
        />
        <Group justify="flex-end" mt="md">
          <FormButton>بروزرسانی</FormButton>
        </Group>
      </Stack>
    </form>
  );
}
