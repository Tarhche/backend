"use client";
import {Box} from "@mantine/core";
import {FormButton} from "@/components/form-button";

export function LogoutForm() {
  return (
    <Box component="form" action={async () => {}}>
      <FormButton variant="outline" color="red" type="submit">
        خروج از حساب
      </FormButton>
    </Box>
  );
}
