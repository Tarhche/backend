"use client";
import {useFormState} from "react-dom";
import Link from "next/link";
import {
  Alert,
  Anchor,
  TextInput,
  Paper,
  Title,
  Text,
  Stack,
  Box,
} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {FieldErrors} from "./field-errors";
import {IconInfoCircle} from "@tabler/icons-react";
import {resetPassword} from "../actions/reset-password";

type Props = {
  token: string;
};

export function ResetPasswordForm({token}: Props) {
  const [state, dispatch] = useFormState(resetPassword, null);

  return (
    <Box pt={60}>
      <Paper withBorder shadow="md" p={30} radius="md">
        <Title ta="center">تغییر کلمه عبور</Title>
        <Text c="dimmed" size="sm" ta="center" mt={5}>
          کلمه عبور جدیدتان را وارد کنید
        </Text>
        <form action={dispatch}>
          <Stack gap={8}>
            <TextInput
              label="کلمه عبور جدید"
              placeholder="..."
              name="new-password"
              mt={"md"}
              error={Boolean(state?.fieldErrors?.password)}
              disabled={state?.success}
              required
            />
            <FieldErrors errors={[state?.fieldErrors?.password ?? ""]} />
          </Stack>
          <Stack gap={8}>
            <TextInput
              label="تکرار کلمه عبور جدید"
              placeholder="..."
              name="confirm-new-password"
              mt={"sm"}
              error={Boolean(state?.fieldErrors?.password)}
              disabled={state?.success}
              required
            />
            <FieldErrors errors={[state?.fieldErrors?.password ?? ""]} />
          </Stack>
          <input name="token" value={token} hidden readOnly />
          {state?.success === true && (
            <Alert
              variant="filled"
              color="green"
              title="ثبت نام موفق"
              mt={"sm"}
              icon={<IconInfoCircle />}
            >
              کلمه عبور شما با موفقیت تغییر یافت. میتوانید به صفحه{" "}
              <Anchor
                c={"white"}
                underline="always"
                component={Link}
                href={"/auth/login"}
              >
                ورود
              </Anchor>{" "}
              مراجعه کنید و وارد حساب خود شوید
            </Alert>
          )}
          {state?.success === false && (
            <>
              {state.errorMessage?.map?.((err) => {
                return (
                  <Alert
                    key={err}
                    variant="filled"
                    color="red"
                    title="عملیات ناموفق"
                    mt={"sm"}
                    icon={<IconInfoCircle />}
                  >
                    {err}
                  </Alert>
                );
              })}
            </>
          )}
          <FormButton mt="lg" type="submit" disabled={state?.success} fullWidth>
            {state?.success === false ? "تلاش مجدد" : "تغییر کلمه عبور"}
          </FormButton>
        </form>
      </Paper>
    </Box>
  );
}
