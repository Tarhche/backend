"use client";
import {useFormState} from "react-dom";
import {Stack, Group, TextInput} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {updateProfilePasswordAction} from "../../actions/update-password";

export function ProfilePasswordForm() {
  const [state, dispatch] = useFormState(updateProfilePasswordAction, {
    success: true,
  });

  return (
    <form action={dispatch}>
      <Stack>
        <TextInput
          name="current_password"
          label="کلمه عبور کنونی"
          error={state.fieldErrors?.current_password || ""}
        />
        <TextInput
          name="new_password"
          label="کلمه عبور جدید"
          error={state.fieldErrors?.new_password || ""}
        />
        <Group justify="flex-end" mt="md">
          <FormButton>تغییر کلمه عبور</FormButton>
        </Group>
      </Stack>
    </form>
  );
}
