<template>
	<div class="container">
		<div class="row">
			<dashboardSidebar class="col-md-3 ml-sm-auto"/>
			<main class="col-md-9 ml-sm-auto">

				<nav aria-label="breadcrumb">
					<ol class="breadcrumb">
						<li class="breadcrumb-item">
							<NuxtLink to="/dashboard">داشبورد</NuxtLink>
						</li>
						<li class="breadcrumb-item">
							<NuxtLink to="/dashboard/profile">پروفایل</NuxtLink>
						</li>
						<li class="breadcrumb-item active" aria-current="page">تغییر کلمه عبور</li>
					</ol>
				</nav>

                <form @submit.prevent="updatePassword()" class="card">
                    <div class="card-body">
                        <div class="row mb-3">
                            <label for="password" class="col-sm-2 col-form-label">کلمه عبور فعلی</label>
                            <div class="col-sm-10">
                                <input :class="{ 'is-invalid': errors.currentPassword }" type="password" placeholder="کلمه عبور فعلی" class="form-control" id="password" v-model="params.currentPassword">
                                <div v-if="errors.currentPassword" class="invalid-feedback">
                                    {{ errors.currentPassword }}
                                </div>
                            </div>
                        </div>

                        <div class="row mb-3">
                            <label for="new_password" class="col-sm-2 col-form-label">کلمه عبور جدید</label>
                            <div class="col-sm-10">
                                <input :class="{ 'is-invalid': errors.newPassword }" type="password" placeholder="کلمه عبور جدید" class="form-control" id="new_password" v-model="params.newPassword">
                                <div v-if="errors.newPassword" class="invalid-feedback">
                                    {{ errors.newPassword }}
                                </div>
                            </div>
                        </div>

                        <div class="row mb-3">
                            <label for="repassword" class="col-sm-2 col-form-label">تکرار کلمه عبور جدید</label>
                            <div class="col-sm-10">
                                <input :class="{ 'is-invalid': errors.newRePassword }" type="password" placeholder="تکرار کلمه عبور جدید" class="form-control" id="repassword" v-model="params.newRePassword">
                                <div v-if="errors.newRePassword" class="invalid-feedback">
                                    {{ errors.newRePassword }}
                                </div>
                            </div>
                        </div>
                    </div>
                    <div class="card-footer">
                        <button :disabled="params.loading" type="submit" class="btn btn-primary rounded submit px-3">
                            <span v-if="!params.loading">تغییر کلمه عبور</span>
                            <div v-else class="spinner-border" role="status">
                                <span class="visually-hidden">Loading...</span>
                            </div>
                        </button>
                    </div>
                </form>

			</main>
		</div>
	</div>
</template>

<script lang="ts" setup>
definePageMeta({
	layout: 'dashboard',
})

useHead({
	title: "تغییر کلمه عبور"
})

// reflects form parameters
const params = reactive({
    currentPassword: null,
    newPassword: null,
    newRePassword: null,
    loading: false,
})

// reflects the validation errors to corresponding html input.
const errors = reactive({
    currentPassword: null,
    newPassword: null,
    newRePassword: null,
})

function resetErrors() {
    errors.currentPassword = null
    errors.newPassword = null
    errors.newRePassword = null
}

async function updatePassword() {
    resetErrors()

    if (!params.currentPassword || params.currentPassword.length == 0) {
        errors.currentPassword = "پسوورد فعلی را وارد کنید"

        return
    }
    
    if (! params.newPassword || params.newPassword.length == 0) {
        errors.newPassword = "پسوورد جدید را وارد کنید"

        return
    }
    
    if (params.newRePassword != params.newPassword) {
        errors.newRePassword = "پسوورد جدید و تکرار آن باید یکسان باشند"

        return
    }

	params.loading = true

	try {
		await useUser().updatePassword(
            params.currentPassword,
            params.newPassword,
		)
	} catch (error) {
		console.log(error)

        if (error.response.status == 400) {
            errors.currentPassword = "کلمه عبور وارد شده اشتباه است"
        }
	}

	params.loading = false
}
</script>
