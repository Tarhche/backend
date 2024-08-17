<template>
  <div class="container">
    <div class="row justify-content-center ">
      <div class="col-md-12 col-lg-10">
        <div class="d-flex">
          <div class="w-100 d-none d-md-block" id="register-cover"></div>
          <div class="w-100 mt-3 mt-0 p-4">
            <h3 class="mb-4 text-center ">ثبت نام</h3>
            <form action="#" class="signin-form d-flex flex-column" @submit.prevent="register()">
              <svg xmlns="http://www.w3.org/2000/svg" style="display: none;">
                <symbol id="check-circle-fill" fill="currentColor" viewBox="0 0 16 16">
                  <path d="M16 8A8 8 0 1 1 0 8a8 8 0 0 1 16 0zm-3.97-3.03a.75.75 0 0 0-1.08.022L7.477 9.417 5.384 7.323a.75.75 0 0 0-1.06 1.06L6.97 11.03a.75.75 0 0 0 1.079-.02l3.992-4.99a.75.75 0 0 0-.01-1.05z"/>
                </symbol>
                <symbol id="info-fill" fill="currentColor" viewBox="0 0 16 16">
                  <path d="M8 16A8 8 0 1 0 8 0a8 8 0 0 0 0 16zm.93-9.412-1 4.705c-.07.34.029.533.304.533.194 0 .487-.07.686-.246l-.088.416c-.287.346-.92.598-1.465.598-.703 0-1.002-.422-.808-1.319l.738-3.468c.064-.293.006-.399-.287-.47l-.451-.081.082-.381 2.29-.287zM8 5.5a1 1 0 1 1 0-2 1 1 0 0 1 0 2z"/>
                </symbol>
                <symbol id="exclamation-triangle-fill" fill="currentColor" viewBox="0 0 16 16">
                  <path d="M8.982 1.566a1.13 1.13 0 0 0-1.96 0L.165 13.233c-.457.778.091 1.767.98 1.767h13.713c.889 0 1.438-.99.98-1.767L8.982 1.566zM8 5c.535 0 .954.462.9.995l-.35 3.507a.552.552 0 0 1-1.1 0L7.1 5.995A.905.905 0 0 1 8 5zm.002 6a1 1 0 1 1 0 2 1 1 0 0 1 0-2z"/>
                </symbol>
              </svg>

              <div>
                <div class="alert alert-info">
                  <h4 class="alert-heading">
                    <svg class="bi flex-shrink-0 m-2" width="24" height="24" role="img" aria-label="Info:"><use xlink:href="#info-fill"/></svg>
                    <span>توجه کنید</span>
                  </h4>
                  <hr>
                  <ul class="small">
                    <li>بعد از ثبت نام یک پیام در ایمیل خود دریافت خواهید کرد و با استفاده از آن میتوانید باقی مراحل ثبت
                      نام را انجام دهید.
                    </li>
                    <li>برای ثبت نام از ایمیل خود استفاده کنید.</li>
                    <li>چنانچه مراحل ثبت نام را قبلا کامل کرده اید میتوانید
                      <nuxt-link to="/auth/login">وارد حساب کاربری</nuxt-link>
                      خود شوید.
                    </li>
                  </ul>
                </div>
              </div>
              <section class="alert alert-success text-center" v-if="params.showNextSteps">
                <p class="mb-0">
                  <svg class="bi flex-shrink-0 mx-2" width="24" height="24" role="img" aria-label="Success:"><use xlink:href="#check-circle-fill"/></svg>
                  <span>لینک ثبت نام برای شما ارسال شد. لطفا ایمیل خود را بررسی کنید.</span>
                </p>
              </section>
              <section v-else>
                <div class="form-group my-2">
                  <input :class="{ 'is-invalid': errors.identity }" type="text" placeholder="ایمیل"
                         class="input form-control py-2 " v-model="params.identity" required>
                  <div v-if="errors.identity" class="invalid-feedback">
                    {{ errors.identity }}
                  </div>
                </div>
                <div class="form-group">
                  <button :disabled="params.loading" type="submit"
                          class="form-control btn btn-primary rounded submit px-3">
                    <span v-if="!params.loading">ثبت نام</span>
                    <div v-else class="spinner-border" role="status">
                      <span class="visually-hidden">Loading...</span>
                    </div>
                  </button>
                </div>
                <div
                    class="form-group d-flex flex-column flex-md-row  mt-2 pt-2 justify-content-md-end gap-3 align-items-md-center">
                  <div>
                    <nuxt-link to="/auth/forgot-password" class="btn btn-outline-danger w-100 btn-sm ">بازیابی کلمه عبور
                    </nuxt-link>
                  </div>
                  <div>
                    <nuxt-link to="/auth/login" class="btn btn-outline-success w-100 btn-sm ">ورود</nuxt-link>
                  </div>
                </div>
              </section>
            </form>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
useHead({
  name: 'ثبت نام',
  meta: [
    {name: 'description', content: 'ثبت نام'},
  ],
  link: [
    {rel: 'canonical', href: `/auth/register`}
  ]
})

// reflects form parameters
const params = reactive({
  identity: null,
  loading: false,
  showNextSteps: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
  identity: null,
})

async function register() {
  try {
    params.loading = true
    await useAuth().register(params.identity)
    params.showNextSteps = true
  } catch (error) {
    console.log(error)
    errors.identity = "شما قبلا ثبت نام کرده اید"
  }

  params.loading = false
}
</script>


<style scoped>
#register-cover {
  background-image: url('/img/register-bg.jpg');
  background-size: cover;
  background-repeat: no-repeat;
  background-position: center top;
  overflow: hidden;
  border-radius: 3px;
}

h3 {
  color: #313131;
}

input::placeholder {
  color: #9a9999;
  font-size: 0.9rem;
}
</style>
