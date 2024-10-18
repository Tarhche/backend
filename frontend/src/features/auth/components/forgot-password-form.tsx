"use client";
import {useFormState} from "react-dom";
import {
  TextInput,
  Paper,
  Title,
  Text,
  Stack,
  Container,
  Alert,
} from "@mantine/core";
import {FormButton} from "@/components/form-button";
import {FieldErrors} from "./field-errors";
import {IconInfoCircle} from "@tabler/icons-react";
import {forgotPassword} from "../actions/forgot-password";

export function ForgotPasswordForm() {
  const [state, dispatch] = useFormState(forgotPassword, undefined);

  return (
    <Container size={500} my={80}>
      <Paper withBorder shadow="md" p={30} mt={30} radius="md">
        <Title ta="center">بازیابی کلمه عبور</Title>
        <Text c="dimmed" size="sm" ta="center" mt={5}>
          با ایمیل یا نام کاربری تان میتوانید کلمه عبورتان را تغییر دهید
        </Text>
        <form action={dispatch}>
          <Stack gap={8}>
            <TextInput
              label="ایمیل یا نام کاربری"
              placeholder="you@email.com"
              name="identity"
              mt={"md"}
              error={Boolean(state?.fieldErrors?.identity)}
              disabled={state?.success}
              required
            />
            <FieldErrors errors={[state?.fieldErrors?.identity ?? ""]} />
          </Stack>
          {state?.success === true && (
            <Alert
              variant="filled"
              color="green"
              title="عملیات موفق"
              mt={"sm"}
              icon={<IconInfoCircle />}
            >
              لینک بازیابی کلمه عبور با موفقیت برای شما ارسال شد. لطفا ایمیل خود
              را بررسی کنید
            </Alert>
          )}
          {state?.success === false && (
            <>
              {state.errorMessages?.map?.((err) => {
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
          <FormButton mt="sm" type="submit" disabled={state?.success} fullWidth>
            درخواست {state?.success === false ? "مجدد" : "بازیابی"}
          </FormButton>
        </form>
      </Paper>
    </Container>
  );
}
