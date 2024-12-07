"use client";
import {useFormState} from "react-dom";
import {Paper, Stack, Group, TextInput} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {updateUserPasswordAction} from "../../actions/change-password";

type Props = {
  userId: string;
};

export function UserPasswordForm({userId}: Props) {
  const [state, dispatch] = useFormState(updateUserPasswordAction, {
    success: true,
  });

  return (
    <Paper withBorder p="xl">
      <form action={dispatch}>
        <Stack>
          <TextInput
            label="کلمه عبور"
            name="password"
            error={state.fieldErrors?.password}
          />
          <TextInput
            label="تکرار کلمه عبور"
            name="repassword"
            error={state.fieldErrors?.rePassword}
          />
          <Group justify="flex-end" mt={"lg"}>
            <FormButton>تغییر کلمه عبور</FormButton>
          </Group>
          <input type="text" name="userId" value={userId} readOnly hidden />
        </Stack>
      </form>
    </Paper>
  );
}
