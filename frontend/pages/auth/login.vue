<template>
  <div class="container">
    <div class="row justify-content-center ">
      <div class="col-md-12 col-lg-10">
        <div class="wrap d-md-flex">
          <div class="img"></div>
          <div class="login-wrap mt-3 mt-md-0  p-md-4">
            <div class="d-flex">
              <div class="w-100 ">
                <h3 class="mb-4 text-center ">ورود</h3>
              </div>
            </div>
            <form action="#" class="signin-form d-flex flex-column " @submit.prevent="handleSubmit">
              <div class="form-group mt-2  position-relative">
                <label class="label user-select-none" for="name">نام کاربری :</label>
                <input type="text" class=" input form-control py-2 " @keyup="removeError" v-model="userName">
                <span class="error" ref="userNameError">لطفا کادر بالا را پر کنید .</span>
              </div>
              <div class="form-group my-4  position-relative">
                <label class="label user-select-none" for="password"> کلمه عبور :</label>
                <input type="password" class=" input form-control py-2 " @keyup="removeError" v-model="password">
                <span class="error" ref="passwordError">لطفا کادر بالا را پر کنید .</span>
              </div>
              <div class="form-group">
                <button type="submit" class="form-control btn btn-primary rounded submit px-3"> ورود</button>
              </div>
              <div class="form-group d-flex  flex-sm-row  mt-2 pt-2 justify-content-between align-items-center">
                <div class=" text-left">
                  <label class="checkbox-wrap d-flex align-items-center gap-1 checkbox-primary mb-0 ">
                    <input type="checkbox" checked>
                    <span class="checkmark"></span>
                    من رو به خاطر بسپار.
                  </label>
                </div>
                <div class=" ">
                  <nuxt-link to="/auth/forgot-password" class="btn btn-outline-danger w-100 btn-sm ">فراموشی رمز عبور</nuxt-link>
                </div>
              </div>
            </form>
            <!--                         <p class="text-center">Not a member? <a data-toggle="tab" href="#signup">Sign Up</a></p>-->
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
<script setup>
import {onMounted} from "vue";
import {useTarcheApi} from '~/store/tarche.js'
import axios from "axios";
const store = useTarcheApi()
const userName = ref("")
const password = ref("")
const userNameError = ref(null)
const passwordError = ref(null)
 const handleSubmit = async() => {
  if (!userName.value.length && !password.value.length) {
    userNameError.value.style.display = "block"
    passwordError.value.style.display = "block"
  } else if (!userName.value.length) {
    userNameError.value.style.display = "block"
  } else if (!password.value.length) {
    passwordError.value.style.display = "block"
  } else {
    const {data:data , error} = await useFetch( "https://tarhche-backend.liara.run/api/auth/login", {
      method:"POST" ,
      body:{
        "username": "mahdi.khanzadi",
        "password": "123"
      }

    })
    console.log(data ,error)
    userNameError.value.style.display = "none"
    passwordError.value.style.display = "none"
  }
}

const removeError = () => {
  if (userName.value.length) {
    userNameError.value.style.display = "none"
  }
  if (password.value.length) {
    passwordError.value.style.display = "none"
  }
}

onMounted(() => {
  const inputs = document.querySelectorAll(".input")
  const placeholders = document.querySelectorAll(".label")
  inputs.forEach((input, index) => {
    input.addEventListener('click', () => {
      placeholders[index].classList.add('transform')
    })
  })
  inputs.forEach((input, index) => {
    input.addEventListener('blur', () => {
      if (inputs[index].value.length === 0) {
        placeholders[index].classList.remove('transform')

      }
    })
  })
  placeholders.forEach((item, index) => {
    item.addEventListener('click', () => {
      placeholders[index].classList.add('transform')
      inputs[index].focus()
    })
  })
})

/*
  function handleCreateProfile() {
  if(internationalCode.value.length !== 10){
  errorInternationalCode.value=true
  } else if(lastName.value.length === 0){
  errorLastName.value=true
  } else if(lastName.value.length === 0 ){
  errorDate.value=true
  }
  else{
    errorInternationalCode.value=false
    errorLastName.value=false
    errorDate.value=false
    setCookie(token)
    Toast.success(   'ثبت نام با موفقیت انجام شد 😘' , {
      timeout: 1400,

    })
    setTimeout(()=>    router.go(-1)
    , 1500)
  }
}
function setCookie(token){
  let time = new Date();
  document.cookie = `sheLife=${token} ; expires=${time.setTime(
      time.getTime() + 60 * 60 * 24 * 365 * 1000
  )};path=/ `
}

function getCookie(fullName) {
  const name = `${fullName}=`;
  let getCookie = document.cookie.split(";");
  for (let i = 0; i < getCookie.length; i++) {
    let string = getCookie[i].trim();
    string.indexOf(name);
    if (string.indexOf(name) != -1) {
      let cookie = string.split('=');
      if(cookie[0]===fullName){
        login.value.status = true
        return cookie[1];
      }
    else {
        login.value.status = false
      }
    }
  }
}

getCookie(token)
 */





</script>
<style scoped>
.container {
  min-height: calc(100vh - 175px);
}

.wrap {
  margin-top: 10vh;
}

.img, .login-wrap {
  width: 50%;
}

.img {
  background-image: url('/img/login-bg.jpg');
  background-size: cover;
  background-repeat: no-repeat;
  background-position: center top;
  overflow: hidden;
  border-radius: 3px;
}

h3 {
  color: #313131;
}


.label {
  position: absolute;
  top: 24px;
  transform: translate(-15%, -50%);
  color: #9a9999;
  transition: 0.5s;
  font-size: 0.9rem;
}

.label:hover {
  cursor: pointer;
}

.label.transform {
  top: 0;
  background: #ffffff;
  padding: 0 5px;
  color: #313131;
  font-size: 1rem;
}

input:not([type="checkbox"]) {
  padding: 0.7rem !important;
  font-size: 0.9rem;
}

input:focus {
  box-shadow: none;
}

.login-wrap {
  position: relative;
}

input[type="checkbox"] {
  display: none;
}

.checkmark {
  display: flex;
  justify-content: center;
  align-items: center;
  width: 20px;
  height: 20px;
  border: 1px solid #eee;
  border-radius: 3px;
  //overflow: hidden;
  transition: 0.3s 0.3s;
}

.checkmark::after {
  content: "";
  position: absolute;
  display: inline-block;
  width: 5px;
  height: 13px;
  border: 3px solid #fff;
  border-top: 0;
  border-left: 0;
  transform: rotate(40deg) translate(10px, 10px);
  transition: 0.3s;
  margin-bottom: 1px;
}

input[type="checkbox"]:checked + .checkmark {
  background: #0994eb;
  transition: 0.3s;
}

input[type="checkbox"]:checked + .checkmark::after {
  transform: rotate(45deg) translate(0);
  transition: 0.3s 0.4s;

}

.error {
  color: #f86262;
  margin-top: 4px;
  margin-right: 0.5rem;
  font-size: 0.7rem;
  display: none;
}

@media (max-width: 991.98px) {
  .img, .login-wrap {
    width: 100%;
  }
}

@media (max-width: 767.98px) {
  .wrap .img {
    height: 250px;
  }
  .login-wrap{
    padding-top: 0;
  }
}

@media (max-width: 350px) {
  .wrap .img {
    height: 250px;
  }

  div a.btn {
    font-size: 0.8rem;
  }

  label.checkbox-wrap {
    font-size: 0.8rem;
  }
}
</style>